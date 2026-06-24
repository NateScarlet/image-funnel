package note

import (
	"main/internal/shared"
	"main/internal/util"
)

func NewFilterBuilder() *FilterBuilder {
	return &FilterBuilder{}
}

type FilterBuilder struct {
}

func (fb *FilterBuilder) Build(filters shared.NoteFilters) func(*Note) bool {
	var b util.FilterBuilder[*Note]

	if v := filters.ID; v != nil {
		m := util.AddToSet(nil, v...)
		b.Add(func(note *Note) bool {
			return m.Has(note.ID())
		})
	}

	if v := filters.Hidden; v != nil {
		hidden := *v
		b.Add(func(note *Note) bool {
			return note.Hidden() == hidden
		})
	}

	return b.Build()
}