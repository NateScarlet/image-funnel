package localfs

import (
	"time"

	"main/internal/shared"
)

// #region 版本1（已废弃，使用 filterMemoBy）
// 注意：v1 版本已废弃，仅用于向后兼容读取旧版 state 文件

// DirectoryStateDTOV1 v1版本的目录状态数据传输对象
type DirectoryStateDTOV1 struct {
	Version     int                                `json:"version"`
	Browse      *DirectoryStateBrowseDTOV1         `json:"browse,omitempty"`
	LastSession *DirectoryStateLastSessionDTOAlias `json:"lastSession,omitempty"`
	UpdatedAt   time.Time                          `json:"updatedAt"`
}

// DirectoryStateBrowseDTOV1 v1版本的状态中的过滤条件配置（包含已废弃的 filterMemoBy）
type DirectoryStateBrowseDTOV1 struct {
	FilterBy     *shared.ImageFilters `json:"filterBy,omitempty"`     // ImageFilters v1
	FilterMemoBy *shared.NoteFilters  `json:"filterMemoBy,omitempty"` // NoteFilters v1（已重命名为 filterNoteBy）
}

// DirectoryStateLastSessionDTOAlias v1版本的 LastSession
// ID 使用 any：早期版本 scalar.ID 未实现 JSON 序列化，写入的 id 可能是 {}，需像 v2 一样容错迁移
type DirectoryStateLastSessionDTOAlias struct {
	ID         any `json:"id"`
	Filter     any `json:"filter"`
	TargetKeep int `json:"targetKeep"`
}

// #endregion
