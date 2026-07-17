package shared

import (
	"time"

	"main/internal/scalar"
)

// MarkImageOptions 包含标记图片时的可选参数
type MarkImageOptions struct {
	duration scalar.Duration
}

// MarkImageOption 是用于设置 MarkImageOptions 的函数类型
type MarkImageOption func(*MarkImageOptions)

// NewMarkImageOptions 创建一个新的 MarkImageOptions 实例
func NewMarkImageOptions(opts ...MarkImageOption) *MarkImageOptions {
	o := &MarkImageOptions{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// WithDuration 设置操作耗时
func WithDuration(d scalar.Duration) MarkImageOption {
	return func(o *MarkImageOptions) {
		o.duration = d
	}
}

// Duration 获取操作耗时
func (o *MarkImageOptions) Duration() scalar.Duration {
	return o.duration
}

// #region SendNotificationOptions

// SendNotificationOptions 发送通知的可选参数
type SendNotificationOptions struct {
	NotAfter   time.Time
	NotBefore  time.Time
	DetailsURL scalar.URI
}

// SendNotificationOption 是用于设置 SendNotificationOptions 的函数类型
type SendNotificationOption func(*SendNotificationOptions)

// NewSendNotificationOptions 创建一个新的 SendNotificationOptions 实例
func NewSendNotificationOptions(opts ...SendNotificationOption) *SendNotificationOptions {
	o := &SendNotificationOptions{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// WithNotAfter 设置过期时间
func WithNotAfter(t time.Time) SendNotificationOption {
	return func(o *SendNotificationOptions) { o.NotAfter = t }
}

// WithNotBefore 设置最早可见时间
func WithNotBefore(t time.Time) SendNotificationOption {
	return func(o *SendNotificationOptions) { o.NotBefore = t }
}

// WithDetailsURL 设置详情 URL
func WithDetailsURL(u scalar.URI) SendNotificationOption {
	return func(o *SendNotificationOptions) { o.DetailsURL = u }
}

// #endregion

// #region UpdateNotificationOptions

// UpdateNotificationOptions 更新通知内容的可选参数
type UpdateNotificationOptions struct {
	Title      string
	Body       string
	Priority   NotificationPriority
	NotAfter   time.Time
	NotBefore  time.Time
	DetailsURL scalar.URI
	UpdatedAt  time.Time
}

// UpdateNotificationOption 是用于设置 UpdateNotificationOptions 的函数类型
type UpdateNotificationOption func(*UpdateNotificationOptions)

// NewUpdateNotificationOptions 创建一个新的 UpdateNotificationOptions 实例
func NewUpdateNotificationOptions(opts ...UpdateNotificationOption) *UpdateNotificationOptions {
	o := &UpdateNotificationOptions{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// WithUpdateTitle 设置标题
func WithUpdateTitle(title string) UpdateNotificationOption {
	return func(o *UpdateNotificationOptions) { o.Title = title }
}

// WithUpdateBody 设置正文
func WithUpdateBody(body string) UpdateNotificationOption {
	return func(o *UpdateNotificationOptions) { o.Body = body }
}

// WithUpdatePriority 设置优先级
func WithUpdatePriority(p NotificationPriority) UpdateNotificationOption {
	return func(o *UpdateNotificationOptions) { o.Priority = p }
}

// WithUpdateNotAfter 设置过期时间
func WithUpdateNotAfter(t time.Time) UpdateNotificationOption {
	return func(o *UpdateNotificationOptions) { o.NotAfter = t }
}

// WithUpdateNotBefore 设置最早可见时间
func WithUpdateNotBefore(t time.Time) UpdateNotificationOption {
	return func(o *UpdateNotificationOptions) { o.NotBefore = t }
}

// WithUpdateDetailsURL 设置详情 URL
func WithUpdateDetailsURL(u scalar.URI) UpdateNotificationOption {
	return func(o *UpdateNotificationOptions) { o.DetailsURL = u }
}

// WithUpdateTime 设置更新时间
func WithUpdateTime(t time.Time) UpdateNotificationOption {
	return func(o *UpdateNotificationOptions) { o.UpdatedAt = t }
}

// #endregion
