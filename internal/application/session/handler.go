package session

import (
	"context"
	"fmt"
	"iter"
	"main/internal/domain/directory"
	"main/internal/domain/session"
)

type Handler struct {
	sessionRepo session.Repository
	dirScanner  directory.Scanner
	eventBus    EventBus
	urlSigner   URLSigner
}

func NewHandler(
	sessionRepo session.Repository,
	dirScanner directory.Scanner,
	eventBus EventBus,
	urlSigner URLSigner,
) *Handler {
	return &Handler{
		sessionRepo: sessionRepo,
		dirScanner:  dirScanner,
		eventBus:    eventBus,
		urlSigner:   urlSigner,
	}
}

func (h *Handler) CreateSession(
	ctx context.Context,
	directoryId string,
	filter *ImageFilters,
	targetKeep int,
) (string, error) {
	path, err := directory.DecodeDirectoryID(directoryId)
	if err != nil {
		return "", err
	}
	images, err := h.dirScanner.Scan(path)
	if err != nil {
		return "", fmt.Errorf("failed to scan directory: %w", err)
	}

	domainImages := make([]*session.Image, len(images))
	for i, img := range images {
		domainImages[i] = session.NewImage(
			img.ID(),
			img.Filename(),
			img.Path(),
			img.Size(),
			img.CurrentRating(),
			img.XMPExists(),
		)
	}

	sess := session.NewSession(directoryId, toDomainFilter(filter), targetKeep, domainImages)

	if err := h.sessionRepo.Save(sess); err != nil {
		return "", fmt.Errorf("failed to save session: %w", err)
	}

	h.eventBus.PublishSession(ctx, h.toSessionDTO(sess))

	return sess.ID(), nil
}

func (h *Handler) MarkImage(
	ctx context.Context,
	sessionID string,
	imageID string,
	action Action,
) error {
	sess, err := h.sessionRepo.FindByID(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	if err := sess.MarkImage(imageID, toDomainAction(action)); err != nil {
		return fmt.Errorf("failed to mark image: %w", err)
	}

	if err := h.sessionRepo.Save(sess); err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	h.eventBus.PublishSession(ctx, h.toSessionDTO(sess))

	return nil
}

func (h *Handler) Undo(ctx context.Context, sessionID string) error {
	sess, err := h.sessionRepo.FindByID(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	if err := sess.Undo(); err != nil {
		return fmt.Errorf("failed to undo: %w", err)
	}

	if err := h.sessionRepo.Save(sess); err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	h.eventBus.PublishSession(ctx, h.toSessionDTO(sess))

	return nil
}

func (h *Handler) Commit(
	ctx context.Context,
	sessionID string,
	keepRating int,
	pendingRating int,
	rejectRating int,
) (int, []error) {
	sess, err := h.sessionRepo.FindByID(sessionID)
	if err != nil {
		return 0, []error{fmt.Errorf("session not found: %w", err)}
	}

	writeActions := session.NewWriteActions(keepRating, pendingRating, rejectRating)
	success, errors := sess.Commit(writeActions)

	if err := h.sessionRepo.Save(sess); err != nil {
		return 0, []error{fmt.Errorf("failed to save session: %w", err)}
	}

	h.eventBus.PublishSession(ctx, h.toSessionDTO(sess))

	return success, errors
}

func (h *Handler) GetSession(ctx context.Context, sessionID string) (*SessionDTO, error) {
	sess, err := h.sessionRepo.FindByID(sessionID)
	if err != nil {
		return nil, err
	}

	return h.toSessionDTO(sess), nil
}

func (h *Handler) GetCurrentImage(ctx context.Context, sessionID string) (*ImageDTO, error) {
	sess, err := h.sessionRepo.FindByID(sessionID)
	if err != nil {
		return nil, err
	}

	img := sess.CurrentImage()
	if img == nil {
		return nil, nil
	}

	return h.toImageDTO(img), nil
}

func (h *Handler) GetSessionStats(ctx context.Context, sessionID string) (*StatsDTO, error) {
	sess, err := h.sessionRepo.FindByID(sessionID)
	if err != nil {
		return nil, err
	}

	return h.toStatsDTO(sess.Stats()), nil
}

func toDomainFilter(filter *ImageFilters) *session.ImageFilters {
	if filter == nil {
		return session.NewImageFilters(nil)
	}
	return session.NewImageFilters(filter.Rating)
}

func toDTOFilter(filter *session.ImageFilters) *ImageFilters {
	if filter == nil {
		return &ImageFilters{Rating: nil}
	}
	return &ImageFilters{
		Rating: filter.Rating(),
	}
}

func toDomainAction(action Action) session.Action {
	return session.Action(action)
}

func toDTOAction(action session.Action) Action {
	return Action(action)
}

func (h *Handler) toSessionDTO(sess *session.Session) *SessionDTO {
	return &SessionDTO{
		ID:         sess.ID(),
		Directory:  sess.Directory(),
		Filter:     toDTOFilter(sess.Filter()),
		TargetKeep: sess.TargetKeep(),
		Status:     Status(sess.Status()),
		Stats:      h.toStatsDTO(sess.Stats()),
		CreatedAt:  sess.CreatedAt(),
		UpdatedAt:  sess.UpdatedAt(),
		CanCommit:  sess.CanCommit(),
		CanUndo:    sess.CanUndo(),
		CurrentImage: func() *ImageDTO {
			if img := sess.CurrentImage(); img != nil {
				return h.toImageDTO(img)
			}
			return nil
		}(),
		QueueStatus: h.toQueueStatusDTO(sess),
	}
}

func (h *Handler) toImageDTO(img *session.Image) *ImageDTO {
	url, _ := h.urlSigner.GenerateSignedURL(img.Path())
	return &ImageDTO{
		ID:            img.ID(),
		Filename:      img.Filename(),
		Size:          img.Size(),
		URL:           url,
		CurrentRating: img.Rating(),
		XMPExists:     img.XMPExists(),
	}
}

func (h *Handler) toStatsDTO(stats *session.Stats) *StatsDTO {
	return &StatsDTO{
		Total:     stats.Total(),
		Processed: stats.Processed(),
		Kept:      stats.Kept(),
		Reviewed:  stats.Reviewed(),
		Rejected:  stats.Rejected(),
		Remaining: stats.Remaining(),
	}
}

func (h *Handler) toQueueStatusDTO(sess *session.Session) *QueueStatusDTO {
	stats := sess.Stats()
	progress := float64(0)
	if stats.Total() > 0 {
		progress = float64(stats.Processed()) / float64(stats.Total()) * 100
	}

	return &QueueStatusDTO{
		CurrentIndex: sess.CurrentIndex(),
		TotalImages:  stats.Total(),
		CurrentImage: func() *ImageDTO {
			if img := sess.CurrentImage(); img != nil {
				return h.toImageDTO(img)
			}
			return nil
		}(),
		Progress: progress,
	}
}

func (h *Handler) SubscribeSession(ctx context.Context) iter.Seq2[*SessionDTO, error] {
	return h.eventBus.SubscribeSession(ctx)
}
