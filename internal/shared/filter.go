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

// IsZero 判断所有筛选字段是否均为零值（切片长度为 0 且其余字段为零值）。
// 用于目录状态合并时判定「传入的筛选条件是否为空」，刻意沿用原有的 len 判定：
// 此处的零长度切片表示「该字段无配置、不覆盖旧值」，与 FilterBuilder 中
// 「非 nil 空切片表示 0 匹配」的筛选语义不同，不可混用。
// 新增字段时必须同步在此处添加判定，shared 包的字段遗漏守卫测试会捕获遗漏。
func (f ImageFilters) IsZero() bool {
	return len(f.ID) == 0 &&
		len(f.DirectoryID) == 0 &&
		len(f.Rating) == 0 &&
		len(f.Label) == 0 &&
		f.Query == ""
}

// NoteFilters 笔记过滤条件
type NoteFilters struct {
	ID          []scalar.ID `json:"id,omitempty"`          // 按笔记ID过滤，空表示不过滤
	DirectoryID []scalar.ID `json:"directoryId,omitempty"` // 按所在目录ID过滤，空表示不过滤
	Hidden      *bool       `json:"hidden,omitempty"`      // 是否被隐藏
}
