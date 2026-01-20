package directory

import (
	"context"
	"iter"

	"main/internal/domain/image"
)

type Scanner interface {
	// TODO:  Add context to Scan methods
	Scan(relPath string) iter.Seq2[*image.Image, error]
	ScanDirectories(relPath string) iter.Seq2[*DirectoryInfo, error]
	AnalyzeDirectory(ctx context.Context, relPath string) (*DirectoryStats, error)
}
