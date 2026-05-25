package image

import (
	"main/internal/shared"
	"main/internal/util"
	"strings"
)

func NewFilterBuilder() *FilterBuilder {
	return &FilterBuilder{}
}

type FilterBuilder struct {
}

func (fb *FilterBuilder) Build(filter shared.ImageFilters) func(*Image) bool {
	var b util.FilterBuilder[*Image]

	if v := filter.ID; len(v) > 0 {
		m := util.AddToSet(nil, v...)
		b.Add(func(img *Image) bool {
			return m.Has(img.ID())
		})
	}

	if v := filter.DirectoryID; len(v) > 0 {
		m := util.AddToSet(nil, v...)
		b.Add(func(img *Image) bool {
			return m.Has(img.DirectoryID())
		})
	}

	if v := filter.Rating; len(v) > 0 {
		m := util.AddToSet(nil, v...)
		b.Add(func(img *Image) bool {
			return m.Has(img.Rating())
		})
	}

	if v := filter.Label; len(v) > 0 {
		labels := make(map[string]bool, len(v))
		for _, l := range v {
			labels[strings.ToLower(l)] = true
		}
		b.Add(func(img *Image) bool {
			return labels[strings.ToLower(img.Label())]
		})
	}

	if filter.Query != "" {
		queryLower := strings.ToLower(filter.Query)
		b.Add(func(img *Image) bool {
			return strings.Contains(strings.ToLower(img.Filename()), queryLower)
		})
	}

	return b.Build()
}