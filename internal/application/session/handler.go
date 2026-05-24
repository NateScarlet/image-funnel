package session

import (
	"context"
	"fmt"
	"iter"
	appimage "main/internal/application/image"
	"main/internal/domain/session"
	"main/internal/scalar"
	"main/internal/shared"
	"time"

	"go.uber.org/zap"
)

type Handler struct {
	sessionService  *session.Service
	eventBus        EventBus
	dtoFactory      *SessionDTOFactory
	imageDTOFactory *appimage.ImageDTOFactory
	logger          *zap.Logger
}

func NewHandler(
	sessionService *session.Service,
	eventBus EventBus,
	dtoFactory *SessionDTOFactory,
	imageDTOFactory *appimage.ImageDTOFactory,
	logger *zap.Logger,
) *Handler {
	return &Handler{
		sessionService:  sessionService,
		eventBus:        eventBus,
		dtoFactory:      dtoFactory,
		imageDTOFactory: imageDTOFactory,
		logger:          logger,
	}
}

func (h *Handler) CreateSession(
	ctx context.Context,
	id scalar.ID,
	directoryId scalar.ID,
	filter *shared.ImageFilters,
	target_keep int,
) (err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("create session failed",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did create session",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	return h.sessionService.Create(ctx, id, directoryId, filter, target_keep)
}

func (h *Handler) MarkImage(
	ctx context.Context,
	sessionID scalar.ID,
	imageID scalar.ID,
	action shared.ImageAction,
	options ...shared.MarkImageOption,
) (err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("mark image failed",
				zap.Stringer("sessionID", sessionID),
				zap.Stringer("imageID", imageID),
				zap.Stringer("action", action),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did mark image",
				zap.Stringer("sessionID", sessionID),
				zap.Stringer("imageID", imageID),
				zap.Stringer("action", action),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	return h.sessionService.MarkImage(ctx, sessionID, imageID, action, options...)
}

func (h *Handler) Undo(ctx context.Context, sessionID scalar.ID) (err error) {
	startTime := time.Now()
	defer func() {
		if err != nil {
			h.logger.Error("undo failed",
				zap.Stringer("sessionID", sessionID),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did undo",
				zap.Stringer("sessionID", sessionID),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	return h.sessionService.Undo(ctx, sessionID)
}

func (h *Handler) Commit(
	ctx context.Context,
	sessionID scalar.ID,
	keepRating int,
	shelveRating int,
	rejectRating int,
) (success int, err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("commit session failed",
				zap.Stringer("sessionID", sessionID),
				zap.Duration("duration", time.Since(startTime)),
				zap.Int("success", success),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did commit session",
				zap.Stringer("sessionID", sessionID),
				zap.Duration("duration", time.Since(startTime)),
				zap.Int("success", success),
			)
		}
	}()

	sess, release, err := h.sessionService.Acquire(ctx, sessionID)
	if err != nil {
		return 0, fmt.Errorf("session not found: %w", err)
	}
	defer release()

	writeActions := &shared.WriteActions{
		KeepRating:   keepRating,
		ShelveRating: shelveRating,
		RejectRating: rejectRating,
	}
	return h.sessionService.Commit(ctx, sess, writeActions)
}

func (h *Handler) Session(ctx context.Context, sessionID scalar.ID) (*shared.SessionDTO, error) {
	sess, release, err := h.sessionService.Acquire(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	defer release()

	return h.dtoFactory.New(sess)
}

func (h *Handler) CurrentImage(ctx context.Context, sessionID scalar.ID) (*shared.ImageDTO, error) {
	sess, release, err := h.sessionService.Acquire(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	defer release()

	img := sess.CurrentImage()
	if img == nil {
		return nil, nil
	}

	return h.imageDTOFactory.New(img)
}

func (h *Handler) SessionStats(ctx context.Context, sessionID scalar.ID) (*shared.StatsDTO, error) {
	sess, release, err := h.sessionService.Acquire(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	defer release()

	return sess.Stats(), nil
}

func (h *Handler) SubscribeSession(ctx context.Context) iter.Seq2[*shared.SessionDTO, error] {
	return h.eventBus.SubscribeSession(ctx)
}

func (h *Handler) NextImages(ctx context.Context, sessionID scalar.ID, count int) ([]*shared.ImageDTO, error) {
	sess, release, err := h.sessionService.Acquire(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	defer release()

	images := sess.NextImages(count)
	if len(images) == 0 {
		return nil, nil
	}

	result := make([]*shared.ImageDTO, 0, len(images))
	for _, img := range images {
		dto, err := h.imageDTOFactory.New(img)
		if err != nil {
			return nil, err
		}
		result = append(result, dto)
	}
	return result, nil
}

func (h *Handler) KeptImages(ctx context.Context, sessionID scalar.ID, limit, offset int) ([]*shared.ImageDTO, error) {
	sess, release, err := h.sessionService.Acquire(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	defer release()

	images := sess.KeptImages(limit, offset)
	if len(images) == 0 {
		return nil, nil
	}

	result := make([]*shared.ImageDTO, 0, len(images))
	for _, img := range images {
		dto, err := h.imageDTOFactory.New(img)
		if err != nil {
			return nil, err
		}
		result = append(result, dto)
	}
	return result, nil
}

// UpdateSession 更新会话配置
func (h *Handler) UpdateSession(
	ctx context.Context,
	sessionID scalar.ID,
	targetKeep *int,
	filter *shared.ImageFilters,
) (err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("update session failed",
				zap.Stringer("sessionID", sessionID),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did update session",
				zap.Stringer("sessionID", sessionID),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	var options []session.UpdateOption

	if targetKeep != nil {
		options = append(options, session.WithTargetKeep(*targetKeep))
	}

	if filter != nil {
		options = append(options, session.WithFilter(filter))
	}

	return h.sessionService.Update(ctx, sessionID, options...)
}

// UpdateLabel 更新图片的标签
func (h *Handler) UpdateLabel(ctx context.Context, sessionID scalar.ID, imageID scalar.ID, label string) (dto *shared.ImageDTO, err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("update label faield", zap.Stringer("sessionID", sessionID), zap.Stringer("imageID", imageID), zap.Duration("duration", time.Since(startTime)), zap.Error(err))
		} else {
			h.logger.Info("did update label", zap.Stringer("sessionID", sessionID), zap.Stringer("imageID", imageID), zap.Duration("duration", time.Since(startTime)))
		}
	}()

	img, err := h.sessionService.UpdateLabel(ctx, sessionID, imageID, label)
	if err != nil {
		return nil, err
	}

	return h.imageDTOFactory.New(img)
}

// LastSession 获取指定目录下最后更新的会话 DTO
func (h *Handler) LastSession(ctx context.Context, directoryID scalar.ID) (dto *shared.SessionDTO, err error) {
	sess, release, err := h.sessionService.LastSession(ctx, directoryID)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, nil
	}
	defer release()

	return h.dtoFactory.New(sess)
}
