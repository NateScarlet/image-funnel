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

// SendNotificationOptions 发送通知的可选参数，不可变
type SendNotificationOptions struct {
	tag        string
	notAfter   time.Time
	notBefore  time.Time
	detailsURL scalar.URI
	body       string
	priority   NotificationPriority
}

// SendNotificationOption 是用于设置 SendNotificationOptions 的函数类型
type SendNotificationOption func(*SendNotificationOptions)

// NewSendNotificationOptions 创建一个新的 SendNotificationOptions 实例，提供默认值
func NewSendNotificationOptions(opts ...SendNotificationOption) *SendNotificationOptions {
	o := &SendNotificationOptions{
		priority: NotificationPriorityNormal,
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// WithTag 设置通知标签。未指定时由服务端自动生成 UUID。
func WithTag(tag string) SendNotificationOption {
	return func(o *SendNotificationOptions) { o.tag = tag }
}

// WithNotAfter 设置过期时间
func WithNotAfter(t time.Time) SendNotificationOption {
	return func(o *SendNotificationOptions) { o.notAfter = t }
}

// WithNotBefore 设置最早可见时间
func WithNotBefore(t time.Time) SendNotificationOption {
	return func(o *SendNotificationOptions) { o.notBefore = t }
}

// WithDetailsURL 设置详情 URL
func WithDetailsURL(u scalar.URI) SendNotificationOption {
	return func(o *SendNotificationOptions) { o.detailsURL = u }
}

// WithBody 设置正文
func WithBody(body string) SendNotificationOption {
	return func(o *SendNotificationOptions) { o.body = body }
}

// WithPriority 设置优先级
func WithPriority(p NotificationPriority) SendNotificationOption {
	return func(o *SendNotificationOptions) { o.priority = p }
}

// Getters
func (o *SendNotificationOptions) Tag() string            { return o.tag }
func (o *SendNotificationOptions) NotAfter() time.Time    { return o.notAfter }
func (o *SendNotificationOptions) NotBefore() time.Time   { return o.notBefore }
func (o *SendNotificationOptions) DetailsURL() scalar.URI { return o.detailsURL }
func (o *SendNotificationOptions) Body() string           { return o.body }
func (o *SendNotificationOptions) Priority() NotificationPriority {
	return o.priority
}

// #endregion

// #region UpdateNotificationOptions

// UpdateNotificationOptions 更新通知内容的可选参数，不可变
type UpdateNotificationOptions struct {
	title      string
	body       string
	priority   NotificationPriority
	notAfter   time.Time
	notBefore  time.Time
	detailsURL scalar.URI
	updatedAt  time.Time
	readAt      *time.Time // nil = 不修改
	dismissedAt *time.Time // nil = 不修改
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
	return func(o *UpdateNotificationOptions) { o.title = title }
}

// WithUpdateBody 设置正文
func WithUpdateBody(body string) UpdateNotificationOption {
	return func(o *UpdateNotificationOptions) { o.body = body }
}

// WithUpdatePriority 设置优先级
func WithUpdatePriority(p NotificationPriority) UpdateNotificationOption {
	return func(o *UpdateNotificationOptions) { o.priority = p }
}

// WithUpdateNotAfter 设置过期时间
func WithUpdateNotAfter(t time.Time) UpdateNotificationOption {
	return func(o *UpdateNotificationOptions) { o.notAfter = t }
}

// WithUpdateNotBefore 设置最早可见时间
func WithUpdateNotBefore(t time.Time) UpdateNotificationOption {
	return func(o *UpdateNotificationOptions) { o.notBefore = t }
}

// WithUpdateDetailsURL 设置详情 URL
func WithUpdateDetailsURL(u scalar.URI) UpdateNotificationOption {
	return func(o *UpdateNotificationOptions) { o.detailsURL = u }
}

// WithUpdateTime 设置更新时间
func WithUpdateTime(t time.Time) UpdateNotificationOption {
	return func(o *UpdateNotificationOptions) { o.updatedAt = t }
}

// WithUpdateReadAt 设置已读时间
func WithUpdateReadAt(t time.Time) UpdateNotificationOption {
	return func(o *UpdateNotificationOptions) { o.readAt = &t }
}

// WithUpdateDismissedAt 设置关闭时间（同时标记已读）
func WithUpdateDismissedAt(t time.Time) UpdateNotificationOption {
	return func(o *UpdateNotificationOptions) { o.dismissedAt = &t }
}

// Getters
func (o *UpdateNotificationOptions) Title() string                     { return o.title }
func (o *UpdateNotificationOptions) Body() string                      { return o.body }
func (o *UpdateNotificationOptions) Priority() NotificationPriority    { return o.priority }
func (o *UpdateNotificationOptions) NotAfter() time.Time               { return o.notAfter }
func (o *UpdateNotificationOptions) NotBefore() time.Time              { return o.notBefore }
func (o *UpdateNotificationOptions) DetailsURL() scalar.URI            { return o.detailsURL }
func (o *UpdateNotificationOptions) UpdatedAt() time.Time              { return o.updatedAt }
func (o *UpdateNotificationOptions) ReadAt() *time.Time                { return o.readAt }
func (o *UpdateNotificationOptions) DismissedAt() *time.Time           { return o.dismissedAt }

// #endregion