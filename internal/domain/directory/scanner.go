package directory

import (
	"context"
	"iter"

	"main/internal/domain/image"
	"main/internal/domain/memo"
)

type Scanner interface {
	Scan(ctx context.Context, relPath string) iter.Seq2[*image.Image, error]
	LookupImage(ctx context.Context, relPath string) (*image.Image, error)

	ScanDirectories(ctx context.Context, relPath string) iter.Seq2[*Directory, error]
	AnalyzeDirectory(ctx context.Context, relPath string) (*DirectoryStats, error)

	// ScanMemos 扫描指定相对路径目录下的所有备忘录
	ScanMemos(ctx context.Context, relPath string) iter.Seq2[*memo.Memo, error]
}
