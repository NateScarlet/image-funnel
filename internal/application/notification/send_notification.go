package notification

import (
	"context"

	"main/internal/scalar"
	"main/internal/shared"

	"go.uber.org/zap"
)

// SendNotification 发送或覆盖通知
func (h *Handler) SendNotification(
	ctx context.Context,
	tag string,
	channel string,
	title string,
	body string,
	priority shared.NotificationPriority,
	opts ...shared.SendNotificationOption,
) (id scalar.ID, didCreate bool, err error) {
	defer func() {
		if err != nil {
			h.logger.Error("send notification failed",
				zap.String("tag", tag),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did send notification",
				zap.String("tag", tag),
				zap.Bool("didCreate", didCreate),
			)
		}
	}()

	result, err := h.service.SendNotification(ctx, tag, channel, title, body, priority, opts...)
	if err != nil {
		return scalar.ID{}, false, err
	}

	eventType := shared.NotificationEventTypeSent
	if !result.DidCreate {
		eventType = shared.NotificationEventTypeUpdated
	}
	h.topic.Publish(ctx, &shared.NotificationChangedEventDTO{
		Event:        eventType,
		Notification: h.dtoFactory.New(result.Notification),
	})

	return result.Notification.ID(), result.DidCreate, nil
}