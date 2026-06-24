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

// NoteFilters 笔记过滤条件
type NoteFilters struct {
	ID          []scalar.ID `json:"id,omitempty"`          // 按笔记ID过滤，空表示不过滤
	DirectoryID []scalar.ID `json:"directoryId,omitempty"` // 按所在目录ID过滤，空表示不过滤
	Hidden      *bool       `json:"hidden,omitempty"`      // 是否被隐藏
}
