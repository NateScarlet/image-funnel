package image

import (
	"context"
	"iter"
	"main/internal/shared"
)

type Repository interface {
	Get(ctx context.Context, relPath string) (*Image, error)
	Find(ctx context.Context, relPath string) iter.Seq2[*Image, error]
}

// Mover 负责根据过滤条件物理移动图片及其伴随文件（如元数据、描述文本等）
type Mover interface {
	// Move 移动匹配过滤规则的图片到目标目录中
	Move(ctx context.Context, relPath string, filterBy shared.ImageFilters, toDirRelPath string) (movedCount int, targetAbsDir string, err error)
}
