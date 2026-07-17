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
	KeepRating   *int `json:"keepRating"`
	ShelveRating *int `json:"shelveRating"`
	RejectRating *int `json:"rejectRating"`
}

// ImageMeta 图片元数据
type ImageMeta struct {
	Width  int
	Height int
}

// NoteDTO 笔记数据传输对象
type NoteDTO struct {
	ID         scalar.ID
	RelPath    string
	AbsPath    string
	Content    string
	RawContent string
	Hidden     bool
	ModTime    time.Time
}

// NoteConnectionDTO 笔记连接数据传输对象
type NoteConnectionDTO struct {
	Edges    []*NoteEdgeDTO
	Nodes    []*NoteDTO
	PageInfo *PageInfoDTO
}

// NoteEdgeDTO 笔记边数据传输对象
type NoteEdgeDTO struct {
	Node   *NoteDTO
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
	Message             string
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
	Default     *DirectoryStateDefaultDTO     `json:"default,omitempty"`
	UpdatedAt   time.Time                     `json:"updatedAt"`
}

// DirectoryStateBrowseDTO 状态中的图片与笔记过滤条件配置
type DirectoryStateBrowseDTO struct {
	FilterBy     *ImageFilters `json:"filterBy,omitempty"`
	FilterNoteBy *NoteFilters  `json:"filterNoteBy,omitempty"`
}

// DirectoryStateLastSessionDTO 状态中的最近会话配置
type DirectoryStateLastSessionDTO struct {
	ID         scalar.ID    `json:"id"`
	Filter     ImageFilters `json:"filter"`
	TargetKeep int          `json:"targetKeep"`
}

// DirectoryStateDefaultDTO 前端自主管理的默认操作状态
type DirectoryStateDefaultDTO struct {
	WriteActions *WriteActions `json:"writeActions,omitempty"`
}

// UndoTrashResultDTO 撤销回收站的结果数据传输对象
type UndoTrashResultDTO struct {
	RestoredCount    int
	ConflictCount    int
	ConflictDirName  string
	ClientMutationID *string
}

// AutocompleteSuggestionDTO 自动完成建议数据传输对象
type AutocompleteSuggestionDTO struct {
	Text        string
	DisplayText string
	Description string
	Type        string
	Style       string
}

// HookDirectiveDTO 外部钩子提供的笔记指令数据传输对象
type HookDirectiveDTO struct {
	Name         string
	Usage        string
	Autocomplete bool
}

// HookDTO 外部钩子配置数据传输对象
type HookDTO struct {
	ID                 scalar.ID
	Name               string
	Description        string
	CanDispatchByImage bool
	CanDispatchByNote  bool
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

// #region Notification DTOs

type NotificationDTO struct {
	ID          scalar.ID            `json:"id"`
	Tag         string               `json:"tag"`
	Channel     string               `json:"channel"`
	Title       string               `json:"title"`
	Body        string               `json:"body"`
	Priority    NotificationPriority `json:"priority"`
	Status      NotificationStatus   `json:"status"`
	ReadAt      time.Time            `json:"readAt"`
	DismissedAt time.Time            `json:"dismissedAt"`
	NotAfter    time.Time            `json:"notAfter"`
	NotBefore   time.Time            `json:"notBefore"`
	CreatedAt   time.Time            `json:"createdAt"`
	UpdatedAt   time.Time            `json:"updatedAt"`
}

type NotificationFilters struct {
	Channel  *string               `json:"channel,omitempty"`
	Status   *NotificationStatus   `json:"status,omitempty"`
	Priority *NotificationPriority `json:"priority,omitempty"`
	Read     *bool                 `json:"read,omitempty"`
}

type NotificationConnectionDTO struct {
	Edges    []*NotificationEdgeDTO `json:"edges"`
	Nodes    []*NotificationDTO     `json:"nodes"`
	PageInfo *PageInfoDTO           `json:"pageInfo"`
}

type NotificationEdgeDTO struct {
	Node   *NotificationDTO `json:"node"`
	Cursor string           `json:"cursor"`
}

type NotificationChannelDTO struct {
	Channel            string           `json:"channel"`
	UnreadCount        int              `json:"unreadCount"`
	LatestNotification *NotificationDTO `json:"latestNotification,omitempty"`
}

type NotificationChangedEventDTO struct {
	Event        NotificationEventType `json:"event"`
	Notification *NotificationDTO      `json:"notification"`
}

// #endregion

