package notification

import (
	"main/internal/shared"
	"main/internal/util"
)

// FilterBuilder 通用的内存通知筛选器构建器
type FilterBuilder struct{}

func NewFilterBuilder() *FilterBuilder {
	return &FilterBuilder{}
}

// Build 根据 DTO 过滤器构建闭包筛选函数
func (fb *FilterBuilder) Build(filters shared.NotificationFilters) func(*Notification) bool {
	var b util.FilterBuilder[*Notification]

	// 1. 按频道过滤
	if v := filters.Channel; v != nil {
		channel := *v
		b.Add(func(n *Notification) bool {
			return n.Channel() == channel
		})
	}

	// 3. 按状态过滤
	if v := filters.Status; v != nil {
		status := *v
		b.Add(func(n *Notification) bool {
			return n.Status() == status
		})
	}

	// 4. 按优先级过滤
	if v := filters.Priority; v != nil {
		priority := *v
		b.Add(func(n *Notification) bool {
			return n.Priority() == priority
		})
	}

	// 5. 按已读状态过滤
	if v := filters.Read; v != nil {
		read := *v
		b.Add(func(n *Notification) bool {
			hasRead := !n.readAt.IsZero()
			return hasRead == read
		})
	}

	return b.Build()
}
