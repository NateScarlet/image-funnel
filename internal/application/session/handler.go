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
	sessionService *session.Service
	eventBus       EventBus
	urlSigner      appimage.URLSigner
	dtoFactory     *SessionDTOFactory
	logger         *zap.Logger
}

func NewHandler(
	sessionService *session.Service,
	eventBus EventBus,
	urlSigner appimage.URLSigner,
	logger *zap.Logger,
) *Handler {
	return &Handler{
		sessionService: sessionService,
		eventBus:       eventBus,
		urlSigner:      urlSigner,
		// TODO: 应该注入而不是创建
		dtoFactory: NewSessionDTOFactory(urlSigner),
		logger:     logger,
	}
}

func (h *Handler) CreateSession(
	ctx context.Context,
	id scalar.ID,
	directoryId scalar.ID,
	filter *shared.ImageFilters,
	target_keep int,
) (err error) {
	h.logger.Info("will create session",
		zap.Stringer("id", id),
		zap.Stringer("directoryId", directoryId),
		zap.Int("targetKeep", target_keep),
	)
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("did create session",
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
) (err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("mark image",
				zap.Stringer("sessionID", sessionID),
				zap.Stringer("imageID", imageID),
				zap.Stringer("action", action),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("mark image",
				zap.Stringer("sessionID", sessionID),
				zap.Stringer("imageID", imageID),
				zap.Stringer("action", action),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	return h.sessionService.MarkImage(ctx, sessionID, imageID, action)
}

func (h *Handler) Undo(ctx context.Context, sessionID scalar.ID) (err error) {
	startTime := time.Now()
	defer func() {
		if err != nil {
			h.logger.Error("undo",
				zap.Stringer("sessionID", sessionID),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("undo",
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
	pendingRating int,
	rejectRating int,
) (success int, errors []error) {
	h.logger.Info("will commit session",
		zap.Stringer("sessionID", sessionID),
		zap.Int("keepRating", keepRating),
		zap.Int("pendingRating", pendingRating),
		zap.Int("rejectRating", rejectRating),
	)
	startTime := time.Now()

	defer func() {
		if len(errors) > 0 {
			h.logger.Warn("did commit session",
				zap.Stringer("sessionID", sessionID),
				zap.Duration("duration", time.Since(startTime)),
				zap.Int("success", success),
				zap.Int("errorCount", len(errors)),
			)
		} else {
			h.logger.Info("did commit session",
				zap.Stringer("sessionID", sessionID),
				zap.Duration("duration", time.Since(startTime)),
				zap.Int("success", success),
			)
		}
	}()

	sess, err := h.sessionService.Get(sessionID)
	if err != nil {
		return 0, []error{fmt.Errorf("session not found: %w", err)}
	}

	writeActions := session.NewWriteActions(keepRating, pendingRating, rejectRating)
	return h.sessionService.Commit(ctx, sess, writeActions)
}

func (h *Handler) Session(ctx context.Context, sessionID scalar.ID) (*shared.SessionDTO, error) {
	sess, err := h.sessionService.Get(sessionID)
	if err != nil {
		return nil, err
	}

	return h.dtoFactory.New(sess)
}

func (h *Handler) CurrentImage(ctx context.Context, sessionID scalar.ID) (*shared.ImageDTO, error) {
	sess, err := h.sessionService.Get(sessionID)
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

func (h *Handler) SessionStats(ctx context.Context, sessionID scalar.ID) (*shared.StatsDTO, error) {
	sess, err := h.sessionService.Get(sessionID)
	if err != nil {
		return nil, err
	}

	statsDTOFactory := NewStatsDTOFactory()
	return statsDTOFactory.New(sess.Stats())
}

func (h *Handler) SubscribeSession(ctx context.Context) iter.Seq2[*shared.SessionDTO, error] {
	return h.eventBus.SubscribeSession(ctx)
}

func (h *Handler) NextImages(ctx context.Context, sessionID scalar.ID, count int) ([]*shared.ImageDTO, error) {
	sess, err := h.sessionService.Get(sessionID)
	if err != nil {
		return nil, err
	}

	images := sess.NextImages(count)
	if len(images) == 0 {
		return nil, nil
	}

	imageDTOFactory := appimage.NewImageDTOFactory(h.urlSigner)
	result := make([]*shared.ImageDTO, 0, len(images))
	for _, img := range images {
		dto, err := imageDTOFactory.New(img)
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
	h.logger.Info("will update session",
		zap.Stringer("sessionID", sessionID),
	)
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("did update session",
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
