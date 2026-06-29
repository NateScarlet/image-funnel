package directory

import (
	"main/internal/domain/image"
	"main/internal/scalar"
	"path/filepath"
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
	id       scalar.ID
	parentID scalar.ID
	relPath  string
}

// FromRepository 从仓库创建目录，根据相对路径生成 ID
// 不要用作构建函数
func FromRepository(relPath string) *Directory {
	// 统一防御性规整：如果传入的相对路径为空字符串，则规范为代表当前目录的 "."
	if relPath == "" {
		relPath = "."
	}
	relPath = filepath.ToSlash(relPath)
	var parentID scalar.ID
	if relPath != "." {
		parentPath := filepath.ToSlash(filepath.Dir(relPath))
		parentID = encodeID(parentPath)
	}
	return &Directory{
		id:       encodeID(relPath),
		parentID: parentID,
		relPath:  relPath,
	}
}

func (d *Directory) ID() scalar.ID {
	return d.id
}

func (d *Directory) ParentID() scalar.ID {
	return d.parentID
}

func (d *Directory) IsRoot() bool {
	return d.relPath == "."
}

func (d *Directory) RelPath() string {
	return d.relPath
}

