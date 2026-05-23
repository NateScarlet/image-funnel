package directory

import (
	"context"
	"iter"

	"main/internal/domain/image"
	"main/internal/domain/memo"
	"main/internal/shared"
)

type Scanner interface {
	Scan(ctx context.Context, relPath string) iter.Seq2[*image.Image, error]
	LookupImage(ctx context.Context, relPath string) (*image.Image, error)

	ScanDirectories(ctx context.Context, relPath string) iter.Seq2[*Directory, error]
	AnalyzeDirectory(ctx context.Context, relPath string) (*DirectoryStats, error)

	// ScanMemos 扫描指定相对路径目录下的所有备忘录
	ScanMemos(ctx context.Context, relPath string) iter.Seq2[*memo.Memo, error]
}

// ImageMover 专职处理文件物理移动操作的接口
type ImageMover interface {
	// MoveImages 移动指定相对路径目录下满足过滤条件的图片及其配套文件至目标相对路径下
	// toDirRelPath 是相对于当前所在目录的路径。
	MoveImages(ctx context.Context, relPath string, filterBy shared.ImageFilters, toDirRelPath string) (movedCount int, targetAbsDir string, err error)
}
