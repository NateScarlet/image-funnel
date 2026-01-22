package directory

import (
	"main/internal/shared"
	"main/internal/util"
)

func NewFilterBuilder() *FilterBuilder {
	return &FilterBuilder{}
}

type FilterBuilder struct {
}

func (fb *FilterBuilder) Build(filters shared.DirectoryFilters) func(*Directory) bool {
	var b util.FilterBuilder[*Directory]
	if v := filters.ID; v != nil {
		var m = util.AddToSet(nil, v...)
		b.Add(func(dir *Directory) bool {
			return m.Has(dir.ID())
		})
	}
	return b.Build()
}
