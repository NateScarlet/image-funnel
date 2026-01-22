package directory

import (
	"context"
	"main/internal/shared"
	"path/filepath"

	"go.uber.org/zap"
)

// EventBus 事件总线接口
type EventBus interface {
	PublishFileChanged(ctx context.Context, event *shared.FileChangedEvent)
}

// Service 目录领域服务
// 负责监听文件变更并转换为应用层事件
type Service struct {
	watcher  Watcher
	eventBus EventBus
	rootDir  string
	logger   *zap.Logger
	repo     Repository
}

// NewService 创建目录服务
func NewService(watcher Watcher, eventBus EventBus, rootDir string, repo Repository, logger *zap.Logger) (*Service, func()) {
	s := &Service{
		watcher:  watcher,
		eventBus: eventBus,
		rootDir:  rootDir,
		logger:   logger,
		repo:     repo,
	}

	// 启动后台监听
	ctx, cancel := context.WithCancel(context.Background())
	go s.watchAndTransform(ctx)

	cleanup := func() {
		cancel()
	}

	return s, cleanup
}

// watchAndTransform 监听文件变更并转换为事件发布
func (s *Service) watchAndTransform(ctx context.Context) {
	for fileChange, err := range s.watcher.Watch(ctx, s.rootDir) {
		if err != nil {
			s.logger.Error("file watch error", zap.Error(err))
			continue
		}

		// 将绝对路径转换为相对路径
		relPath, err := filepath.Rel(s.rootDir, fileChange.absPath)
		if err != nil {
			s.logger.Error("failed to get relative path",
				zap.String("path", fileChange.absPath),
				zap.String("root", s.rootDir),
				zap.Error(err))
			continue
		}

		// 编码目录ID
		dir, err := s.repo.GetByPath(ctx, filepath.Dir(relPath))
		if err != nil {
			s.logger.Error("failed to get directory by path",
				zap.String("path", filepath.Dir(relPath)),
				zap.Error(err))
			continue
		}

		// 构建应用层事件
		event := &shared.FileChangedEvent{
			DirectoryID: dir.ID(),
			RelPath:     relPath,
			Action:      fileChange.action,
			OccurredAt:  fileChange.occurredAt,
		}

		// 发布事件
		s.eventBus.PublishFileChanged(ctx, event)
	}
}
