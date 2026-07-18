package notification

import (
	"context"
	"time"

	"main/internal/scalar"

	"go.uber.org/zap"
)

// MarkRead 标记通知已读
func (h *Handler) MarkRead(ctx context.Context, id scalar.ID, at time.Time) (err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("mark notification read failed",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did mark notification read",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	return h.service.MarkRead(ctx, id, at)
}

// Dismiss 关闭通知（同时标记已读）
func (h *Handler) Dismiss(ctx context.Context, id scalar.ID, at time.Time) (err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("dismiss notification failed",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did dismiss notification",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	return h.service.Dismiss(ctx, id, at)
}