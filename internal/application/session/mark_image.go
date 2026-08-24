package session

import (
	"context"
	"main/internal/scalar"
	"main/internal/shared"
	"time"

	"go.uber.org/zap"
)

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
