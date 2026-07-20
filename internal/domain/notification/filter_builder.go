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
			return n.VisibleAt(t)
		})
	}

	if v := filters.PendingAt; v != nil {
		t := *v
		// notBefore > t：在时间点 t 尚未到达可见时间（未来才可见）
		b.Add(func(n *Notification) bool { return n.NotBefore().After(t) })
	}

	if v := filters.ExpiredAt; v != nil {
		t := *v
		// notAfter < t：在时间点 t 已超出可见截止（已过期）
		b.Add(func(n *Notification) bool { return n.NotAfter().Before(t) })
	}

	return b.Build()
}