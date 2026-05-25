package memo

import (
	"main/internal/shared"
	"main/internal/util"
)

func NewFilterBuilder() *FilterBuilder {
	return &FilterBuilder{}
}

type FilterBuilder struct {
}

func (fb *FilterBuilder) Build(filters shared.MemoFilters) func(*Memo) bool {
	var b util.FilterBuilder[*Memo]

	if v := filters.ID; len(v) > 0 {
		m := util.AddToSet(nil, v...)
		b.Add(func(memo *Memo) bool {
			return m.Has(memo.ID())
		})
	}

	if v := filters.Hidden; v != nil {
		hidden := *v
		b.Add(func(memo *Memo) bool {
			return memo.Hidden() == hidden
		})
	}

	return b.Build()
}