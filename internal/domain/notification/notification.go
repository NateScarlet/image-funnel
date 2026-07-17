package notification

import (
	"time"

	"main/internal/scalar"
	"main/internal/shared"
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
	deleted     bool
}

// Deletion State
func (n *Notification) IsDeleted() bool { return n.deleted }
func (n *Notification) MarkDeleted()    { n.deleted = true }

// Getters
func (n *Notification) ID() scalar.ID                         { return n.id }
func (n *Notification) Tag() string                           { return n.tag }
func (n *Notification) Channel() string                       { return n.channel }
func (n *Notification) Title() string                         { return n.title }
func (n *Notification) Body() string                          { return n.body }
func (n *Notification) Priority() shared.NotificationPriority { return n.priority }
func (n *Notification) ReadAt() time.Time                     { return n.readAt }
func (n *Notification) DismissedAt() time.Time                 { return n.dismissedAt }
func (n *Notification) NotAfter() time.Time                   { return n.notAfter }
func (n *Notification) NotBefore() time.Time                  { return n.notBefore }
func (n *Notification) CreatedAt() time.Time                  { return n.createdAt }
func (n *Notification) UpdatedAt() time.Time                  { return n.updatedAt }

// Status 状态派生: dismissedAt ≠ 0 → DISMISSED，否则 ACTIVE
func (n *Notification) Status() shared.NotificationStatus {
	if !n.dismissedAt.IsZero() {
		return shared.NotificationStatusDismissed
	}
	return shared.NotificationStatusActive
}

// Update 更新通知内容
func (n *Notification) Update(title, body string, priority shared.NotificationPriority, notAfter, notBefore time.Time, at time.Time) {
	n.title = title
	n.body = body
	n.priority = priority
	n.notAfter = notAfter
	n.notBefore = notBefore
	n.updatedAt = at
}

// MarkRead 标记已读
func (n *Notification) MarkRead(at time.Time) {
	n.readAt = at
	n.updatedAt = at
}

// Dismiss 关闭通知
func (n *Notification) Dismiss(at time.Time) {
	n.dismissedAt = at
	if n.readAt.IsZero() {
		n.readAt = at
	}
	n.updatedAt = at
}

// FromRepository 供持久层加载领域对象使用
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
) *Notification {
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
	}
}
