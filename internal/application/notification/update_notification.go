package notification

import (
	"context"
	"time"

	"main/internal/scalar"

	"go.uber.org/zap"
)

// UpdateNotification 更新通知元数据（已读时间、关闭时间）
func (h *Handler) UpdateNotification(
	ctx context.Context,
	id scalar.ID,
	readAt *time.Time,
	dismissedAt *time.Time,
) (err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("update notification failed",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did update notification",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	return h.service.UpdateNotification(ctx, id, readAt, dismissedAt)
}