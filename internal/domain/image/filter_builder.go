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

// Build 根据过滤条件构建针对具体 *Image 的过滤函数
func (fb *FilterBuilder) Build(filter shared.ImageFilters) func(*Image) bool {
	var b util.FilterBuilder[*Image]

	// 过滤掉无效的空指针，避免空指针解引用引发崩溃
	b.Add(func(img *Image) bool {
		return img != nil
	})

	if v := filter.ID; v != nil {
		m := util.AddToSet(nil, v...)
		b.Add(func(img *Image) bool {
			return m.Has(img.ID())
		})
	}

	if v := filter.DirectoryID; v != nil {
		m := util.AddToSet(nil, v...)
		b.Add(func(img *Image) bool {
			return m.Has(img.DirectoryID())
		})
	}

	if v := filter.Rating; v != nil {
		m := util.AddToSet(nil, v...)
		b.Add(func(img *Image) bool {
			return m.Has(img.Rating())
		})
	}

	if v := filter.Label; v != nil {
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
