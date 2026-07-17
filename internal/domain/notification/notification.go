package notification

import (
	"fmt"
	"time"

	"main/internal/scalar"
	"main/internal/shared"

	"github.com/google/uuid"
)

// Notification 表示一条系统通知
type Notification struct {
	id          scalar.ID
	tag         string
	channel     string
	title       string
	body        string
	priority    shared.NotificationPriority
	readAt      time.Time
	dismissedAt time.Time
	notAfter    time.Time
	notBefore   time.Time
	createdAt   time.Time
	updatedAt   time.Time
	detailsURL  scalar.URI
}

// Getters
func (n *Notification) ID() scalar.ID                         { return n.id }
func (n *Notification) Tag() string                           { return n.tag }
func (n *Notification) Channel() string                       { return n.channel }
func (n *Notification) Title() string                         { return n.title }
func (n *Notification) Body() string                          { return n.body }
func (n *Notification) Priority() shared.NotificationPriority { return n.priority }
func (n *Notification) ReadAt() time.Time                     { return n.readAt }
func (n *Notification) DismissedAt() time.Time                { return n.dismissedAt }
func (n *Notification) NotAfter() time.Time                   { return n.notAfter }
func (n *Notification) NotBefore() time.Time                  { return n.notBefore }
func (n *Notification) CreatedAt() time.Time                  { return n.createdAt }
func (n *Notification) UpdatedAt() time.Time                  { return n.updatedAt }
func (n *Notification) DetailsURL() scalar.URI                { return n.detailsURL }

// Status 状态派生: dismissedAt ≠ 0 → DISMISSED，否则 ACTIVE
func (n *Notification) Status() shared.NotificationStatus {
	if !n.dismissedAt.IsZero() {
		return shared.NotificationStatusDismissed
	}
	return shared.NotificationStatusActive
}

// #region 内部 setter（不导出，仅同包内 Service 使用）

func (n *Notification) setTitle(title string) error {
	if title == "" {
		return fmt.Errorf("title is required")
	}
	n.title = title
	return nil
}

func (n *Notification) setBody(body string) {
	n.body = body
}

func (n *Notification) setPriority(p shared.NotificationPriority) {
	n.priority = p
}

func (n *Notification) setNotAfter(t time.Time) {
	n.notAfter = t
}

func (n *Notification) setNotBefore(t time.Time) {
	n.notBefore = t
}

func (n *Notification) setDetailsURL(u scalar.URI) {
	n.detailsURL = u
}

func (n *Notification) setUpdatedAt(t time.Time) {
	n.updatedAt = t
}

// #endregion

// MarkRead 标记已读
func (n *Notification) MarkRead(at time.Time, now time.Time) {
	n.readAt = at
	n.updatedAt = now
}

// Dismiss 关闭通知
func (n *Notification) Dismiss(at time.Time, now time.Time) {
	n.dismissedAt = at
	if !at.IsZero() {
		if n.readAt.IsZero() {
			n.readAt = at
		}
	}
	n.updatedAt = now
}

// FromRepository 供持久层加载领域对象使用，通过 setter 进行校验
func FromRepository(
	id scalar.ID,
	tag string,
	channel string,
	title string,
	body string,
	priority shared.NotificationPriority,
	readAt time.Time,
	dismissedAt time.Time,
	notAfter time.Time,
	notBefore time.Time,
	createdAt time.Time,
	updatedAt time.Time,
	detailsURL scalar.URI,
) (*Notification, error) {
	// 校验：tag 必须是 UUID
	if _, err := uuid.Parse(tag); err != nil {
		return nil, fmt.Errorf("tag must be a valid UUID: %w", err)
	}
	// 校验：title 必填
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}

	return &Notification{
		id:          id,
		tag:         tag,
		channel:     channel,
		title:       title,
		body:        body,
		priority:    priority,
		readAt:      readAt,
		dismissedAt: dismissedAt,
		notAfter:    notAfter,
		notBefore:   notBefore,
		createdAt:   createdAt,
		updatedAt:   updatedAt,
		detailsURL:  detailsURL,
	}, nil
}

// Factory 负责创建新的 Notification 实例
type Factory struct{}

// New 创建新通知，负责校验和默认值
// body 和 priority 通过 opts 传递，默认值由 NewSendNotificationOptions 提供
func (f *Factory) New(
	tag string,
	channel string,
	title string,
	opts ...shared.SendNotificationOption,
) (*Notification, error) {
	// 校验：tag 必须是 UUID，避免无意冲突
	if _, err := uuid.Parse(tag); err != nil {
		return nil, fmt.Errorf("tag must be a valid UUID: %w", err)
	}
	// 校验：title 必填
	if title == "" {
		return nil, fmt.Errorf("title is required")
	}

	options := shared.NewSendNotificationOptions(opts...)

	// priority 必须非零（默认值已在 NewSendNotificationOptions 中提供，为零说明调用者错误覆盖）
	if options.Priority().IsZero() {
		return nil, fmt.Errorf("priority is required")
	}

	now := time.Now()
	// notBefore 默认当前时间
	notBefore := options.NotBefore()
	if notBefore.IsZero() {
		notBefore = now
	}
	// notAfter 默认 notBefore + 7 天
	notAfter := options.NotAfter()
	if notAfter.IsZero() {
		notAfter = notBefore.Add(7 * 24 * time.Hour)
	}

	// ID 基于 tag 生成，确保同 tag 的 ID 稳定
	id := scalar.ToID("notify:" + tag)
	return FromRepository(
		id, tag, channel, title, options.Body(), options.Priority(),
		time.Time{}, time.Time{}, notAfter, notBefore,
		now, now, options.DetailsURL(),
	)
}