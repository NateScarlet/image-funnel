package notification

import (
	"context"
	"time"

	"main/internal/scalar"
	"main/internal/shared"

	"go.uber.org/zap"
)

// Notification 获取单条通知
func (h *Handler) Notification(ctx context.Context, id scalar.ID) (dto *shared.NotificationDTO, err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("get notification failed",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did get notification",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	notif, err := h.repo.Get(ctx, id.String())
	if err != nil {
		return nil, err
	}
	return h.dtoFactory.New(notif), nil
}