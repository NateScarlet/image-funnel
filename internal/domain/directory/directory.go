package directory

import (
	"main/internal/domain/image"
	"main/internal/scalar"
)

type DirectoryInfo struct {
	id                scalar.ID
	path              string
	imageCount        int
	subdirectoryCount int
	latestImage       *image.Image
	ratingCounts      map[int]int
}

func NewDirectoryInfo(path string, imageCount, subdirectoryCount int, latestImage *image.Image, ratingCounts map[int]int) *DirectoryInfo {
	return &DirectoryInfo{
		id:                EncodeID(path),
		path:              path,
		imageCount:        imageCount,
		subdirectoryCount: subdirectoryCount,
		latestImage:       latestImage,
		ratingCounts:      ratingCounts,
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

func (d *DirectoryInfo) LatestImage() *image.Image {
	return d.latestImage
}

func (d *DirectoryInfo) RatingCounts() map[int]int {
	return d.ratingCounts
}
