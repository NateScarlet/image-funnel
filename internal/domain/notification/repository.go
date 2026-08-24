package notification

import (
	"context"
	"iter"

	"main/internal/scalar"
	"main/internal/shared"
)

// FindOptions 包含 Find 选项
type FindOptions struct {
	filter shared.NotificationFilters
}

// Filter 返回 FindOptions 里的只读过滤器
func (o *FindOptions) Filter() shared.NotificationFilters {
	return o.filter
}

// FindOption 定义选项函数
type FindOption func(*FindOptions)

// FindWithFilter 应用 DTO 筛选条件
func FindWithFilter(f shared.NotificationFilters) FindOption {
	return func(o *FindOptions) {
		o.filter = f
	}
}

// NewFindOptions 辅助构造器，收集所有选项
func NewFindOptions(opts ...FindOption) *FindOptions {
	o := &FindOptions{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// ChannelStats 频道统计信息数据
type ChannelStats struct {
	Channel              string
	UnreadCount          int
	LatestNotificationID scalar.ID
}

// Repository 接口，提供通知存储的核心接口
type Repository interface {
	// Save 保存通知（新建或更新）。返回 didCreate 表示是否为新建通知
	Save(ctx context.Context, notif *Notification) (didCreate bool, err error)
	// Get 根据 ID 获取通知，不存在返回 apperror.NewErrDocumentNotFound
	Get(ctx context.Context, id string) (*Notification, error)
	// GetByTag 根据 tag 获取通知，不存在返回 apperror.NewErrDocumentNotFound
	GetByTag(ctx context.Context, tag string) (*Notification, error)
	// Find 遍历所有通知，支持基于 Options 模式粗筛
	Find(ctx context.Context, options ...FindOption) iter.Seq2[*Notification, error]
	// Channels 遍历获取频道及统计数据
	Channels(ctx context.Context) iter.Seq2[*ChannelStats, error]
}
