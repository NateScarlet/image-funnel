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

	result, err := h.service.SendNotification(ctx, tag, channel, title, opts...)
	if err != nil {
		return scalar.ID{}, false, err
	}

	// 通过 pubsub 推送完整通知，实现读写分离
	notif, err := h.repo.Get(ctx, result.ID().String())
	if err != nil {
		return scalar.ID{}, false, err
	}

	eventType := shared.NotificationEventTypeSent
	if !result.DidCreate() {
		eventType = shared.NotificationEventTypeUpdated
	}
	if err := h.topic.Publish(ctx, &shared.NotificationChangedEventDTO{
		Event:        eventType,
		Notification: h.dtoFactory.New(notif),
	}); err != nil {
		return scalar.ID{}, false, err
	}

	return result.ID(), result.DidCreate(), nil
}