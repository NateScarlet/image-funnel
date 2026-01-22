package directory

import (
	"context"
	"iter"
	"main/internal/shared"
	"time"
)

// FileChange 领域对象 - 文件变更
// 不暴露字段，只提供 getter 方法
type FileChange struct {
	absPath    string
	action     shared.FileAction
	occurredAt time.Time
}

// NewFileChange 创建文件变更对象
func NewFileChange(absPath string, action shared.FileAction, occurredAt time.Time) *FileChange {
	return &FileChange{
		absPath:    absPath,
		action:     action,
		occurredAt: occurredAt,
	}
}

// Watcher 文件系统监控器接口
// FileChange 是专门为 Watcher 设计的领域对象
type Watcher interface {
	// Watch 监听指定目录的文件变更
	Watch(ctx context.Context, dir string) iter.Seq2[*FileChange, error]
}
