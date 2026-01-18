package directory

import (
	"main/internal/scalar"
	"time"
)

type DirectoryInfo struct {
	id                 scalar.ID
	path               string
	imageCount         int
	subdirectoryCount  int
	latestImageModTime time.Time
	latestImagePath    string
	ratingCounts       map[int]int
}

func NewDirectoryInfo(path string, imageCount, subdirectoryCount int, latestImageModTime time.Time, latestImagePath string, ratingCounts map[int]int) *DirectoryInfo {
	return &DirectoryInfo{
		id:                 EncodeID(path),
		path:               path,
		imageCount:         imageCount,
		subdirectoryCount:  subdirectoryCount,
		latestImageModTime: latestImageModTime,
		latestImagePath:    latestImagePath,
		ratingCounts:       ratingCounts,
	}
}

func (d *DirectoryInfo) ID() scalar.ID {
	return d.id
}

func (d *DirectoryInfo) Path() string {
	return d.path
}

func (d *DirectoryInfo) ImageCount() int {
	return d.imageCount
}

func (d *DirectoryInfo) SubdirectoryCount() int {
	return d.subdirectoryCount
}

func (d *DirectoryInfo) LatestImageModTime() time.Time {
	return d.latestImageModTime
}

func (d *DirectoryInfo) LatestImagePath() string {
	return d.latestImagePath
}

func (d *DirectoryInfo) RatingCounts() map[int]int {
	return d.ratingCounts
}
