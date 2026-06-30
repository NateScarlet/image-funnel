package localfs

import (
	"time"

	"main/internal/shared"
)

// DirectoryStateDTOV2 v2版本的目录状态数据传输对象
type DirectoryStateDTOV2 struct {
	Version     int                            `json:"version"`
	Browse      *shared.DirectoryStateBrowseDTO `json:"browse,omitempty"`
	LastSession *DirectoryStateLastSessionDTOV2 `json:"lastSession,omitempty"`
	UpdatedAt   time.Time                      `json:"updatedAt"`
}

// DirectoryStateLastSessionDTOV2 v2版本的最近会话配置（包含旧版 commitActions 和 createActions）
type DirectoryStateLastSessionDTOV2 struct {
	ID            any                  `json:"id"`
	Filter        shared.ImageFilters  `json:"filter"`
	TargetKeep    int                  `json:"targetKeep"`
	CommitActions *shared.WriteActions `json:"commitActions,omitempty"`
	CreateActions *shared.WriteActions `json:"createActions,omitempty"`
}
