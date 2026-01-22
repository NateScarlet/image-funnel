package directory

import (
	"main/internal/domain/image"
	"main/internal/scalar"
)

type DirectoryStats struct {
	imageCount        int
	subdirectoryCount int
	latestImage       *image.Image
	ratingCounts      map[int]int
}

func NewDirectoryStats(imageCount, subdirectoryCount int, latestImage *image.Image, ratingCounts map[int]int) *DirectoryStats {
	return &DirectoryStats{
		imageCount:        imageCount,
		subdirectoryCount: subdirectoryCount,
		latestImage:       latestImage,
		ratingCounts:      ratingCounts,
	}
}

func (s *DirectoryStats) ImageCount() int {
	return s.imageCount
}

func (s *DirectoryStats) SubdirectoryCount() int {
	return s.subdirectoryCount
}

func (s *DirectoryStats) LatestImage() *image.Image {
	return s.latestImage
}

func (s *DirectoryStats) RatingCounts() map[int]int {
	return s.ratingCounts
}

type Directory struct {
	id   scalar.ID
	path string
}

// FromRepository 从仓库创建目录
// 不要用作构建函数
func FromRepository(id scalar.ID, path string) *Directory {
	return &Directory{
		id:   id,
		path: path,
	}
}

func (d *Directory) ID() scalar.ID {
	return d.id
}

func (d *Directory) Path() string {
	return d.path
}
