package image

import (
	"main/internal/scalar"
	"main/internal/shared"
	"main/internal/util"
	"strings"
)

// FilterableImage 定义了图片过滤器所依赖的属性接口契约
type FilterableImage interface {
	ID() scalar.ID
	DirectoryID() scalar.ID
	Rating() int
	Label() string
	Filename() string
}

func NewFilterBuilder() *FilterBuilder {
	return &FilterBuilder{}
}

type FilterBuilder struct {
}

func (fb *FilterBuilder) Build(filter shared.ImageFilters) func(FilterableImage) bool {
	var b util.FilterBuilder[FilterableImage]

	// 过滤掉无效的空指针，避免空指针解引用引发崩溃
	b.Add(func(img FilterableImage) bool {
		return img != nil
	})

	if v := filter.ID; v != nil {
		m := util.AddToSet(nil, v...)
		b.Add(func(img FilterableImage) bool {
			return m.Has(img.ID())
		})
	}

	if v := filter.DirectoryID; v != nil {
		m := util.AddToSet(nil, v...)
		b.Add(func(img FilterableImage) bool {
			return m.Has(img.DirectoryID())
		})
	}

	if v := filter.Rating; v != nil {
		m := util.AddToSet(nil, v...)
		b.Add(func(img FilterableImage) bool {
			return m.Has(img.Rating())
		})
	}

	if v := filter.Label; v != nil {
		labels := make(map[string]bool, len(v))
		for _, l := range v {
			labels[strings.ToLower(l)] = true
		}
		b.Add(func(img FilterableImage) bool {
			return labels[strings.ToLower(img.Label())]
		})
	}

	if filter.Query != "" {
		queryLower := strings.ToLower(filter.Query)
		b.Add(func(img FilterableImage) bool {
			return strings.Contains(strings.ToLower(img.Filename()), queryLower)
		})
	}

	return b.Build()
}