package shared

import "main/internal/scalar"

// ImageFilters 图片过滤条件
type ImageFilters struct {
	ID          []scalar.ID `json:"id,omitempty"`          // 按图片ID过滤，空表示不过滤
	DirectoryID []scalar.ID `json:"directoryId,omitempty"` // 按所在目录ID过滤，空表示不过滤
	Rating      []int       `json:"rating,omitempty"`
	Label       []string    `json:"label,omitempty"`
	Query       string      `json:"query,omitempty"`
}

// MemoFilters 备忘录过滤条件
type MemoFilters struct {
	ID          []scalar.ID `json:"id,omitempty"`          // 按备忘录ID过滤，空表示不过滤
	DirectoryID []scalar.ID `json:"directoryId,omitempty"` // 按所在目录ID过滤，空表示不过滤
	Hidden      *bool       `json:"hidden,omitempty"`      // 是否被隐藏
}



