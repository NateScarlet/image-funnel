package directory

import "time"

type DirectoryDTO struct {
	ID                 string
	ParentID           string
	Path               string
	ImageCount         int
	SubdirectoryCount  int
	LatestImageModTime time.Time
	LatestImagePath    string
	RatingCounts       map[int]int
}
