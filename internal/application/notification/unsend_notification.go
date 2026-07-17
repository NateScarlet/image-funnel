package notification

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// UnsendNotification 撤回（删除）通知
func (h *Handler) UnsendNotification(ctx context.Context, tag string) (err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("unsend notification failed",
				zap.String("tag", tag),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did unsend notification",
				zap.String("tag", tag),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	return h.service.UnsendNotification(ctx, tag)
}