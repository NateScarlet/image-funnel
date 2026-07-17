package notification

import (
	"context"
	"time"

	"main/internal/scalar"
	"main/internal/shared"

	"go.uber.org/zap"
)

// UpdateNotification 更新通知元数据（已读时间、关闭时间）
func (h *Handler) UpdateNotification(
	ctx context.Context,
	id scalar.ID,
	readAt *time.Time,
	dismissedAt *time.Time,
) (dto *shared.NotificationDTO, err error) {
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

	notif, err := h.service.UpdateNotification(ctx, id, readAt, dismissedAt)
	if err != nil {
		return nil, err
	}

	dto = h.dtoFactory.New(notif)
	h.topic.Publish(ctx, &shared.NotificationChangedEventDTO{
		Event:        shared.NotificationEventTypeUpdated,
		Notification: dto,
	})

	return dto, nil
}