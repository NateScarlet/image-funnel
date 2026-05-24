package directory

import (
	"main/internal/domain/image"
	"main/internal/scalar"
)

type Stats struct {
	imageCount        int
	subdirectoryCount int
	latestImage       *image.Image
	ratingCounts      map[int]int
}

func NewStats(imageCount, subdirectoryCount int, latestImage *image.Image, ratingCounts map[int]int) *Stats {
	return &Stats{
		imageCount:        imageCount,
		subdirectoryCount: subdirectoryCount,
		latestImage:       latestImage,
		ratingCounts:      ratingCounts,
	}
}

func (s *Stats) ImageCount() int {
	return s.imageCount
}

func (s *Stats) SubdirectoryCount() int {
	return s.subdirectoryCount
}

func (s *Stats) LatestImage() *image.Image {
	return s.latestImage
}

func (s *Stats) RatingCounts() map[int]int {
	return s.ratingCounts
}

type Directory struct {
	id      scalar.ID
	relPath string
}

// FromRepository 从仓库创建目录
// 不要用作构建函数
func FromRepository(id scalar.ID, relPath string) *Directory {
	return &Directory{
		id:      id,
		relPath: relPath,
	}
}

func (d *Directory) ID() scalar.ID {
	return d.id
}

func (d *Directory) RelPath() string {
	return d.relPath
}
