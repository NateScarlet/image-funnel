package session

import (
	"context"
	"fmt"
	"iter"
	appimage "main/internal/application/image"
	"main/internal/domain/directory"
	domainimage "main/internal/domain/image"
	"main/internal/domain/session"
	"main/internal/scalar"
)

type Handler struct {
	sessionRepo    session.Repository
	sessionService *session.Service
	dirScanner     directory.Scanner
	eventBus       EventBus
	urlSigner      appimage.URLSigner
	dtoFactory     *SessionDTOFactory
}

func NewHandler(
	sessionRepo session.Repository,
	sessionService *session.Service,
	dirScanner directory.Scanner,
	eventBus EventBus,
	urlSigner appimage.URLSigner,
) *Handler {
	return &Handler{
		sessionRepo:    sessionRepo,
		sessionService: sessionService,
		dirScanner:     dirScanner,
		eventBus:       eventBus,
		urlSigner:      urlSigner,
		dtoFactory:     NewSessionDTOFactory(urlSigner),
	}
}

func (h *Handler) CreateSession(
	ctx context.Context,
	id scalar.ID,
	directoryId scalar.ID,
	filter *appimage.ImageFilters,
	target_keep int,
) error {
	path, err := directory.DecodeID(directoryId)
	if err != nil {
		return err
	}
	images, err := h.dirScanner.Scan(path)
	if err != nil {
		return fmt.Errorf("failed to scan directory: %w", err)
	}

	domainFilter := toDomainFilter(filter)
	filterFunc := domainimage.BuildImageFilter(domainFilter)

	filteredImages := domainimage.FilterImages(images, filterFunc)

	directory, err := directory.DecodeID(directoryId)
	if err != nil {
		return err
	}
	sess := session.NewSession(id, directory, domainFilter, target_keep, filteredImages)

	if err = h.sessionRepo.Save(sess); err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	sessionDTO, err := h.dtoFactory.New(sess)
	if err != nil {
		return fmt.Errorf("failed to create session DTO: %w", err)
	}
	h.eventBus.PublishSession(ctx, sessionDTO)

	return nil
}

func (h *Handler) MarkImage(
	ctx context.Context,
	sessionID scalar.ID,
	imageID scalar.ID,
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

	sessionDTO, err := h.dtoFactory.New(sess)
	if err != nil {
		return fmt.Errorf("failed to create session DTO: %w", err)
	}
	h.eventBus.PublishSession(ctx, sessionDTO)

	return nil
}

func (h *Handler) Undo(ctx context.Context, sessionID scalar.ID) error {
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

	sessionDTO, err := h.dtoFactory.New(sess)
	if err != nil {
		return fmt.Errorf("failed to create session DTO: %w", err)
	}
	h.eventBus.PublishSession(ctx, sessionDTO)

	return nil
}

func (h *Handler) Commit(
	ctx context.Context,
	sessionID scalar.ID,
	keepRating int,
	pendingRating int,
	rejectRating int,
) (int, []error) {
	sess, err := h.sessionRepo.FindByID(sessionID)
	if err != nil {
		return 0, []error{fmt.Errorf("session not found: %w", err)}
	}

	writeActions := session.NewWriteActions(keepRating, pendingRating, rejectRating)
	success, errors := h.sessionService.Commit(sess, writeActions)

	if err := h.sessionRepo.Save(sess); err != nil {
		return 0, []error{fmt.Errorf("failed to save session: %w", err)}
	}

	sessionDTO, err := h.dtoFactory.New(sess)
	if err != nil {
		return 0, []error{fmt.Errorf("failed to create session DTO: %w", err)}
	}
	h.eventBus.PublishSession(ctx, sessionDTO)

	return success, errors
}

func (h *Handler) GetSession(ctx context.Context, sessionID scalar.ID) (*SessionDTO, error) {
	sess, err := h.sessionRepo.FindByID(sessionID)
	if err != nil {
		return nil, err
	}

	return h.dtoFactory.New(sess)
}

func (h *Handler) GetCurrentImage(ctx context.Context, sessionID scalar.ID) (*appimage.ImageDTO, error) {
	sess, err := h.sessionRepo.FindByID(sessionID)
	if err != nil {
		return nil, err
	}

	img := sess.CurrentImage()
	if img == nil {
		return nil, nil
	}

	imageDTOFactory := appimage.NewImageDTOFactory(h.urlSigner)
	return imageDTOFactory.New(img)
}

func (h *Handler) GetSessionStats(ctx context.Context, sessionID scalar.ID) (*StatsDTO, error) {
	sess, err := h.sessionRepo.FindByID(sessionID)
	if err != nil {
		return nil, err
	}

	statsDTOFactory := NewStatsDTOFactory()
	return statsDTOFactory.New(sess.Stats())
}

func toDomainFilter(filter *appimage.ImageFilters) *domainimage.ImageFilters {
	if filter == nil {
		return domainimage.NewImageFilters(nil)
	}
	return domainimage.NewImageFilters(filter.Rating)
}

func toDTOFilter(filter *domainimage.ImageFilters) *appimage.ImageFilters {
	if filter == nil {
		return &appimage.ImageFilters{Rating: nil}
	}
	return &appimage.ImageFilters{
		Rating: filter.Rating(),
	}
}

func toDomainAction(action Action) session.Action {
	return session.Action(action)
}

func (h *Handler) SubscribeSession(ctx context.Context) iter.Seq2[*SessionDTO, error] {
	return h.eventBus.SubscribeSession(ctx)
}
