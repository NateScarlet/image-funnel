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
func (n *Notification) NotAfter() time.Time {
	if n.notAfter.IsZero() {
		return n.NotBefore().Add(7 * 24 * time.Hour)
	}
	return n.notAfter
}
func (n *Notification) NotBefore() time.Time {
	if n.notBefore.IsZero() {
		return n.createdAt
	}
	return n.notBefore
}
func (n *Notification) CreatedAt() time.Time                  { return n.createdAt }
func (n *Notification) UpdatedAt() time.Time                  { return n.updatedAt }
func (n *Notification) DetailsURL() scalar.URI                { return n.detailsURL }

// VisibleAt 判断通知在给定时间点是否可见
// 使用 !t.Before/!t.After 确保边界相等时与字段字面一致
func (n *Notification) VisibleAt(t time.Time) bool {
	return !t.Before(n.NotBefore()) && !t.After(n.NotAfter())
}

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

func (n *Notification) setBody(body string) error {
	n.body = body
	return nil
}

func (n *Notification) setPriority(p shared.NotificationPriority) error {
	if p.IsZero() {
		return fmt.Errorf("priority is required")
	}
	n.priority = p
	return nil
}

func (n *Notification) setNotAfter(t time.Time) error {
	n.notAfter = t
	return nil
}

func (n *Notification) setNotBefore(t time.Time) error {
	n.notBefore = t
	// 调用者显式调整 notBefore 时，清除在新 notBefore 值之前的 readAt 和 dismissedAt
	if n.readAt.Before(t) {
		n.readAt = time.Time{}
	}
	if n.dismissedAt.Before(t) {
		n.dismissedAt = time.Time{}
	}
	return nil
}

func (n *Notification) setDetailsURL(u scalar.URI) error {
	n.detailsURL = u
	return nil
}

func (n *Notification) setUpdatedAt(t time.Time) error {
	n.updatedAt = t
	return nil
}

// #endregion

// markRead 标记已读（仅同包 Service 使用）
func (n *Notification) markRead(at time.Time, now time.Time) {
	n.readAt = at
	n.updatedAt = now
}

// dismiss 关闭通知（仅同包 Service 使用）
func (n *Notification) dismiss(at time.Time, now time.Time) {
	n.dismissedAt = at
	if !at.IsZero() {
		if n.readAt.IsZero() {
			n.readAt = at
		}
	}
	n.updatedAt = now
}

// #region updateNotificationOption（不导出，仅同包 Service 使用）

// updateNotificationOption 更新通知元数据的选项函数
type updateNotificationOption func(n *Notification, now time.Time)

// withReadAt 设置已读时间。传 nil 表示重置为未读
func withReadAt(t *time.Time) updateNotificationOption {
	return func(n *Notification, now time.Time) {
		if t == nil {
			n.readAt = time.Time{}
		} else {
			n.readAt = *t
		}
		n.updatedAt = now
	}
}

// withDismissedAt 设置关闭时间。传 nil 表示撤销关闭
func withDismissedAt(t *time.Time) updateNotificationOption {
	return func(n *Notification, now time.Time) {
		if t == nil {
			n.dismissedAt = time.Time{}
		} else {
			n.dismissedAt = *t
			if n.readAt.IsZero() {
				n.readAt = *t
			}
		}
		n.updatedAt = now
	}
}

// #endregion

// FromRepository 供持久层加载领域对象使用，不校验（仓库数据已可信）
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
	options := shared.NewSendNotificationOptions(opts...)

	now := time.Now()

	// ID 基于 tag 生成，确保同 tag 的 ID 稳定
	id := scalar.ToID("notify:" + tag)
	n := &Notification{
		id:          id,
		tag:         tag,
		channel:     channel,
		readAt:      time.Time{},
		dismissedAt: time.Time{},
		createdAt:   now,
		updatedAt:   now,
	}
	if err := n.setTitle(title); err != nil {
		return nil, err
	}
	if err := n.setBody(options.Body()); err != nil {
		return nil, err
	}
	if err := n.setPriority(options.Priority()); err != nil {
		return nil, err
	}
	// notBefore/notAfter 默认值由 getter 提供（NotBefore → CreatedAt, NotAfter → NotBefore+7d）
	if err := n.setNotAfter(options.NotAfter()); err != nil {
		return nil, err
	}
	if err := n.setNotBefore(options.NotBefore()); err != nil {
		return nil, err
	}
	if err := n.setDetailsURL(options.DetailsURL()); err != nil {
		return nil, err
	}
	return n, nil
}