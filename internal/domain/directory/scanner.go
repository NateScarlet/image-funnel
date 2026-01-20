package directory

import (
	"main/internal/domain/image"
)

type Scanner interface {
	Scan(dirPath string) ([]*image.Image, error)
	ScanDirectories(relPath string) ([]*DirectoryInfo, error)
	AnalyzeDirectory(relPath string) (int, int, *image.Image, map[int]int, error)
	ValidateDirectoryPath(relPath string) error
}
