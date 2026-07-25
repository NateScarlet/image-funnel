package session

import (
	"context"
	"main/internal/scalar"
	"time"

	"go.uber.org/zap"
)

// TODO: 重命名为 UndoMarkImage
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
