package shared

import (
	"time"

	"main/internal/scalar"
)

// FileChangedEvent 文件变更事件 - 应用层事件，包含目录ID
type FileChangedEvent struct {
	DirectoryID scalar.ID // 变更文件所在的目录ID
	RelPath     string    // 文件路径
	Action      FileAction
	OccurredAt  time.Time
}

// DirectoryFilters 目录查询过滤器
type DirectoryFilters struct {
	ID []scalar.ID // 目录ID列表，空表示所有目录
}

// DirectoryDTO 目录数据传输对象
type DirectoryDTO struct {
	ID       scalar.ID
	ParentID scalar.ID
	RelPath  string
	Root     bool
}

// DirectoryStatsDTO 目录统计数据传输对象
type DirectoryStatsDTO struct {
	ImageCount        int
	SubdirectoryCount int
	LatestImage       *ImageDTO
	RatingCounts      map[int]int
}

// ImageDTO 图片数据传输对象
type ImageDTO struct {
	ID            scalar.ID
	Filename      string
	Size          int64
	AbsPath       string
	RelPath       string
	ModTime       time.Time
	CurrentRating int
	Width         int
	Height        int
	XMPExists     bool
	Label         string
}

// SessionDTO 会话数据传输对象
type SessionDTO struct {
	ID           scalar.ID
	DirectoryID  scalar.ID
	Filter       *ImageFilters
	TargetKeep   int
	Stats        *StatsDTO
	CreatedAt    time.Time
	UpdatedAt    time.Time
	CanCommit    bool
	CanUndo      bool
	CurrentIndex int
	CurrentSize  int
	CurrentRound int
	CurrentImage *ImageDTO
	CurrentRoundActions []ImageAction
}

// StatsDTO 会话统计数据
type StatsDTO struct {
	TotalCount            int
	TotalKept             int
	TotalShelved          int
	TotalRejected         int
	CurrentRoundRemaining int
	IsCompleted           bool
}

// WriteActions 写入操作配置
type WriteActions struct {
	KeepRating   int
	ShelveRating int
	RejectRating int
}

// ImageMeta 图片元数据
type ImageMeta struct {
	Width  int
	Height int
}

// MemoDTO 备忘录数据传输对象
type MemoDTO struct {
	ID         scalar.ID
	RelPath    string
	AbsPath    string
	Content    string
	RawContent string
	Hidden     bool
}

// MemoConnectionDTO 备忘录连接数据传输对象
type MemoConnectionDTO struct {
	Edges    []*MemoEdgeDTO
	Nodes    []*MemoDTO
	PageInfo *PageInfoDTO
}

// MemoEdgeDTO 备忘录边数据传输对象
type MemoEdgeDTO struct {
	Node   *MemoDTO
	Cursor string
}

// ImageConnectionDTO 图片连接数据传输对象
type ImageConnectionDTO struct {
	Edges    []*ImageEdgeDTO
	Nodes    []*ImageDTO
	PageInfo *PageInfoDTO
}

// ImageEdgeDTO 图片边数据传输对象
type ImageEdgeDTO struct {
	Node   *ImageDTO
	Cursor string
}

// PageInfoDTO 页面信息数据传输对象
type PageInfoDTO struct {
	HasNextPage     bool
	HasPreviousPage bool
	StartCursor     string
	EndCursor       string
}

