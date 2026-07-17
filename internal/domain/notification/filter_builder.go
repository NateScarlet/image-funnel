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

	if v := filters.Channel; v != nil {
		ch := *v
		b.Add(func(n *Notification) bool { return n.Channel() == ch })
	}

	if v := filters.Status; v != nil {
		st := *v
		b.Add(func(n *Notification) bool { return n.Status() == st })
	}

	if v := filters.Priority; v != nil {
		p := *v
		b.Add(func(n *Notification) bool { return n.Priority() == p })
	}

	if v := filters.Read; v != nil {
		r := *v
		b.Add(func(n *Notification) bool { return !n.readAt.IsZero() == r })
	}

	if v := filters.NotBefore; v != nil {
		t := *v
		b.Add(func(n *Notification) bool { return n.notBefore.IsZero() || !n.notBefore.After(t) })
	}

	if v := filters.NotAfter; v != nil {
		t := *v
		b.Add(func(n *Notification) bool { return n.notAfter.IsZero() || !n.notAfter.Before(t) })
	}

	return b.Build()
}
