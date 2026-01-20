package shared

import (
	"time"

	"main/internal/scalar"
)

// DirectoryDTO 目录数据传输对象
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

// ImageDTO 图片数据传输对象
type ImageDTO struct {
	ID            scalar.ID
	Filename      string
	Size          int64
	Path          string
	ModTime       time.Time
	CurrentRating int
	Width         int
	Height        int
	XMPExists     bool
}

// SessionDTO 会话数据传输对象
type SessionDTO struct {
	ID           scalar.ID
	Directory    string
	Filter       *ImageFilters
	TargetKeep   int
	Stats        *StatsDTO
	CreatedAt    time.Time
	UpdatedAt    time.Time
	CanCommit    bool
	CanUndo      bool
	CurrentIndex int
	CurrentSize  int
	CurrentImage *ImageDTO
	NextImage    *ImageDTO
}

// StatsDTO 会话统计数据
type StatsDTO struct {
	Total       int
	Kept        int
	Reviewed    int
	Rejected    int
	Remaining   int
	IsCompleted bool
}

// WriteActions 写入操作配置
type WriteActions struct {
	KeepRating    int
	PendingRating int
	RejectRating  int
}
