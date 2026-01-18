package directory

import (
	"main/internal/domain/image"
	"time"
)

type Scanner interface {
	Scan(dirPath string) ([]*image.Image, error)
	ScanDirectories(relPath string) ([]*DirectoryInfo, error)
	AnalyzeDirectory(relPath string) (int, int, time.Time, string, map[int]int, error)
	ValidateDirectoryPath(relPath string) error
}
