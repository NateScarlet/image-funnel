package notification

import (
	"context"

	"main/internal/shared"

	"go.uber.org/zap"
)

// UnsendNotification 撤回通知，标记 notAfter 为当前时间
func (h *Handler) UnsendNotification(ctx context.Context, tag string) (dto *shared.NotificationDTO, err error) {
	defer func() {
		if err != nil {
			h.logger.Error("unsend notification failed",
				zap.String("tag", tag),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did unsend notification",
				zap.String("tag", tag),
			)
		}
	}()

	notif, err := h.service.UnsendNotification(ctx, tag)
	if err != nil {
		return nil, err
	}
	if notif == nil {
		// 通知不存在，什么都不做
		return nil, nil
	}

	dto = h.dtoFactory.New(notif)
	if err := h.topic.Publish(ctx, &shared.NotificationChangedEventDTO{
		Event:        shared.NotificationEventTypeUnsent,
		Notification: dto,
	}); err != nil {
		return nil, err
	}

	return dto, nil
}