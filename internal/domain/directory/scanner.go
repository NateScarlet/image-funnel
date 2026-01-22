package directory

import (
	"context"
	"iter"

	"main/internal/domain/image"
)

type Scanner interface {
	Scan(ctx context.Context, relPath string) iter.Seq2[*image.Image, error]
	LookupImage(ctx context.Context, relPath string) (*image.Image, error)

	ScanDirectories(ctx context.Context, relPath string) iter.Seq2[*Directory, error]
	AnalyzeDirectory(ctx context.Context, relPath string) (*DirectoryStats, error)
}
