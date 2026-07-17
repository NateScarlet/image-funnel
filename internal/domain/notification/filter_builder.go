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

// Build 根据过滤器构建闭包筛选函数
func (fb *FilterBuilder) Build(filters shared.NotificationFilters) func(*Notification) bool {
	var b util.FilterBuilder[*Notification]

	if len(filters.Channel) > 0 {
		chSet := make(map[string]struct{}, len(filters.Channel))
		for _, ch := range filters.Channel {
			chSet[ch] = struct{}{}
		}
		b.Add(func(n *Notification) bool {
			_, ok := chSet[n.Channel()]
			return ok
		})
	}

	if len(filters.Status) > 0 {
		stSet := make(map[shared.NotificationStatus]struct{}, len(filters.Status))
		for _, st := range filters.Status {
			stSet[st] = struct{}{}
		}
		b.Add(func(n *Notification) bool {
			_, ok := stSet[n.Status()]
			return ok
		})
	}

	if len(filters.Priority) > 0 {
		pSet := make(map[shared.NotificationPriority]struct{}, len(filters.Priority))
		for _, p := range filters.Priority {
			pSet[p] = struct{}{}
		}
		b.Add(func(n *Notification) bool {
			_, ok := pSet[n.Priority()]
			return ok
		})
	}

	if v := filters.Read; v != nil {
		r := *v
		b.Add(func(n *Notification) bool { return !n.ReadAt().IsZero() == r })
	}

	if v := filters.VisibleAt; v != nil {
		t := *v
		b.Add(func(n *Notification) bool {
			// notBefore 默认是创建时间，所以不应为零值
			notBefore := n.NotBefore()
			if notBefore.IsZero() {
				notBefore = n.CreatedAt()
			}
			return !notBefore.After(t) &&
				(n.NotAfter().IsZero() || !n.NotAfter().Before(t))
		})
	}

	return b.Build()
}