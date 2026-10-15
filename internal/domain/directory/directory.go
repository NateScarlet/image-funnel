package directory

import (
	"main/internal/domain/image"
	"main/internal/domain/relpath"
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
	id       scalar.ID
	parentID scalar.ID
	relPath  relpath.Path
}

// FromRepository 从仓库创建目录，根据根相对路径生成 ID
// 不要用作构建函数
func FromRepository(relPath relpath.Path) *Directory {
	var parentID scalar.ID
	if !relPath.IsRoot() {
		parentID = encodeID(relPath.Dir())
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
	return d.relPath.IsRoot()
}

// RelPath 返回目录的根相对路径，构造时即保证其位于根目录内
func (d *Directory) RelPath() relpath.Path {
	return d.relPath
}
