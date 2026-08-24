package notification

import (
	"context"
	"iter"

	"main/internal/shared"
)

// SubscribeNotificationChanged 订阅通知全局实时流
func (h *Handler) SubscribeNotificationChanged(ctx context.Context) iter.Seq2[*shared.NotificationChangedEventDTO, error] {
	return h.topic.Subscribe(ctx)
}
