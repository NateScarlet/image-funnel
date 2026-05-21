package shared

import "main/internal/scalar"

// ImageFilters 图片过滤条件
type ImageFilters struct {
	ID          []scalar.ID // 按图片ID过滤，空表示不过滤
	DirectoryID []scalar.ID // 按所在目录ID过滤，空表示不过滤
	Rating      []int
	Label       []string
	Query       string
}

