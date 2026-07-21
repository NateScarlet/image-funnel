package shared

import (
	"context"
)

// NotificationSender 通用通知发送接口
type NotificationSender interface {
	Send(
		ctx context.Context,
		tag string,
		channel string,
		title string,
		opts ...SendNotificationOption,
	) (*SendNotificationResult, error)
}
