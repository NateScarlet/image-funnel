package notification

import (
	"context"
	"time"

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
) (dto *shared.NotificationDTO, didCreate bool, err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("send notification failed",
				zap.String("tag", tag),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did send notification",
				zap.String("tag", tag),
				zap.Bool("didCreate", didCreate),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	result, err := h.service.SendNotification(ctx, tag, channel, title, body, priority, opts...)
	if err != nil {
		return nil, false, err
	}

	dto = h.dtoFactory.New(result.Notification)

	eventType := shared.NotificationEventTypeSent
	if !result.DidCreate {
		eventType = shared.NotificationEventTypeUpdated
	}
	h.topic.Publish(ctx, &shared.NotificationChangedEventDTO{
		Event:        eventType,
		Notification: dto,
	})

	return dto, result.DidCreate, nil
}