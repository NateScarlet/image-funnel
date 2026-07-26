package shared

import (
	"context"
)

// NotificationSender 通用通知发送接口
type NotificationSender interface {
	SendNotification(
		ctx context.Context,
		channel string,
		title string,
		opts ...SendNotificationOption,
	) (*SendNotificationResult, error)
}
