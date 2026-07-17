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
		chSet := util.AddToSet(nil, filters.Channel...)
		b.Add(func(n *Notification) bool {
			return chSet.Has(n.Channel())
		})
	}

	if len(filters.Status) > 0 {
		stSet := util.AddToSet(nil, filters.Status...)
		b.Add(func(n *Notification) bool {
			return stSet.Has(n.Status())
		})
	}

	if len(filters.Priority) > 0 {
		pSet := util.AddToSet(nil, filters.Priority...)
		b.Add(func(n *Notification) bool {
			return pSet.Has(n.Priority())
		})
	}

	if v := filters.Read; v != nil {
		r := *v
		b.Add(func(n *Notification) bool { return !n.ReadAt().IsZero() == r })
	}

	if v := filters.VisibleAt; v != nil {
		t := *v
		b.Add(func(n *Notification) bool {
			return !n.NotBefore().After(t) &&
				(n.NotAfter().IsZero() || !n.NotAfter().Before(t))
		})
	}

	return b.Build()
}