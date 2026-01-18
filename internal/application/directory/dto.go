package directory

import "time"

type DirectoryDTO struct {
	ID                 string
	ParentID           string
	Path               string
	Root               bool
	ImageCount         int
	SubdirectoryCount  int
	LatestImageModTime time.Time
	LatestImagePath    string
	RatingCounts       map[int]int
}
