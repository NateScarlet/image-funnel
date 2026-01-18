package directory

import "time"

type Scanner interface {
	Scan(dirPath string) ([]*ImageInfo, error)
	ScanDirectories(relPath string) ([]*DirectoryInfo, error)
	AnalyzeDirectory(absPath string) (int, int, time.Time, string, map[int]int, error)
	ValidateDirectoryPath(relPath string) error
}
