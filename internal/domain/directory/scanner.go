package directory

import (
	"iter"

	"main/internal/domain/image"
)

type Scanner interface {
	Scan(relPath string) iter.Seq2[*image.Image, error]
	ScanDirectories(relPath string) iter.Seq2[*DirectoryInfo, error]
	AnalyzeDirectory(relPath string) (int, int, *image.Image, map[int]int, error)
	ValidateDirectoryPath(relPath string) error
}
