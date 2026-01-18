package directory

import (
	"time"

	"main/internal/scalar"
)

type DirectoryDTO struct {
	ID                 scalar.ID
	ParentID           scalar.ID
	Path               string
	Root               bool
	ImageCount         int
	SubdirectoryCount  int
	LatestImageModTime time.Time
	LatestImagePath    string
	RatingCounts       map[int]int
}
