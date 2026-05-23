package image

import (
	"context"
	"iter"
	"main/internal/shared"
)

// Scanner 负责扫描和在指定目录中查询图片信息
type Scanner interface {
	// Scan 迭代扫描指定目录下的全部图片
	Scan(ctx context.Context, relPath string) iter.Seq2[*Image, error]
	// Lookup 根据文件相对路径直接查询并构建单个图片对象
	Lookup(ctx context.Context, relPath string) (*Image, error)
}

// Mover 负责根据过滤条件物理移动图片及其伴随文件（如元数据、描述文本等）
type Mover interface {
	// Move 移动匹配过滤规则的图片到目标目录中
	Move(ctx context.Context, relPath string, filterBy shared.ImageFilters, toDirRelPath string) (movedCount int, targetAbsDir string, err error)
}
