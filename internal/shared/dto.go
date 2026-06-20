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

// PathInput 支持多种格式的路径输入
type PathInput struct {
	Absolute          string
	RelativeToRoot    string
	RelativeToCurrent string
}

// DirectoryFilters 目录查询过滤器
type DirectoryFilters struct {
	ID    []scalar.ID // 目录ID列表，空表示所有目录
	Query string      // 按目录名模糊搜索（忽略大小写，包含即匹配）
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

// DirEntryDeletedDTO 被删除或移走的文件/目录数据传输对象
type DirEntryDeletedDTO struct {
	RelPath string
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
	ID                  scalar.ID
	DirectoryID         scalar.ID
	Filter              *ImageFilters
	TargetKeep          int
	Stats               *StatsDTO
	CreatedAt           time.Time
	UpdatedAt           time.Time
	CanCommit           bool
	CanUndo             bool
	CurrentIndex        int
	CurrentSize         int
	CurrentRound        int
	CurrentImageID      scalar.ID
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

// DirectoryConnectionDTO 目录连接数据传输对象
type DirectoryConnectionDTO struct {
	Edges    []*DirectoryEdgeDTO
	Nodes    []*DirectoryDTO
	PageInfo *PageInfoDTO
}

// DirectoryEdgeDTO 目录边数据传输对象
type DirectoryEdgeDTO struct {
	Node   *DirectoryDTO
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

// PairingRequestDTO 配对请求数据传输对象
type PairingRequestDTO struct {
	Code      string
	CreatedAt time.Time
	Status    PairingRequestStatus
}

// DeviceDTO 设备数据传输对象
type DeviceDTO struct {
	ID          scalar.ID
	Name        string
	CreatedAt   time.Time
	LastLoginAt time.Time
	LastLoginIP string
	UserAgent   string
}

// TrashHistoryItemDTO 回收站历史记录数据传输对象
type TrashHistoryItemDTO struct {
	ID                  scalar.ID
	TotalFileCount      int
	TotalFileSize       int64
	TrashedAt           time.Time
	ImageCount          int
	AssociatedFileCount int
	CoverImageAbsPath   string
	SrcRelPath          string
}

// TrashHistoryConnectionDTO 回收站历史连接数据传输对象
type TrashHistoryConnectionDTO struct {
	Edges    []*TrashHistoryEdgeDTO
	Nodes    []*TrashHistoryItemDTO
	PageInfo *PageInfoDTO
}

// TrashHistoryEdgeDTO 回收站历史边数据传输对象
type TrashHistoryEdgeDTO struct {
	Node   *TrashHistoryItemDTO
	Cursor string
}

// DirectoryStateDTO 目录状态数据传输对象
type DirectoryStateDTO struct {
	Version     int                           `json:"version"`
	Browse      *DirectoryStateBrowseDTO      `json:"browse,omitempty"`
	LastSession *DirectoryStateLastSessionDTO `json:"lastSession,omitempty"`
	UpdatedAt   time.Time                     `json:"updatedAt"`
}

// DirectoryStateBrowseDTO 状态中的图片与备忘录过滤条件配置
type DirectoryStateBrowseDTO struct {
	FilterBy     *ImageFilters `json:"filterBy,omitempty"`
	FilterMemoBy *MemoFilters  `json:"filterMemoBy,omitempty"`
}

// DirectoryStateLastSessionDTO 状态中的最近会话配置
type DirectoryStateLastSessionDTO struct {
	ID         scalar.ID    `json:"id"`
	Filter     ImageFilters `json:"filter"`
	TargetKeep int          `json:"targetKeep"`
}

// UndoTrashResultDTO 撤销回收站的结果数据传输对象
type UndoTrashResultDTO struct {
	RestoredCount    int
	ConflictCount    int
	ConflictDirName  string
	ClientMutationID *string
}

// HookDirectiveDTO 外部钩子提供的笔记指令数据传输对象
type HookDirectiveDTO struct {
	Name  string
	Usage string
}

// HookDTO 外部钩子配置数据传输对象
type HookDTO struct {
	ID                 scalar.ID
	Name               string
	Description        string
	CanDispatchByImage bool
	CanDispatchByMemo  bool
	Directive          *HookDirectiveDTO
}

// SessionCommittedEvent 会话提交事件
type SessionCommittedEvent struct {
	SessionID   scalar.ID
	DirectoryID scalar.ID
}

// MetadataUpdatedEvent 图片元数据更新事件
type MetadataUpdatedEvent struct {
	ID        scalar.ID
	Path      string // 绝对物理路径
	Rating    int
	Label     string
	Action    string
	OldRating int
	OldLabel  string
	OldAction string
}



