package session

import (
	"context"
	"main/internal/domain/image"
	"main/internal/shared"
	"main/internal/util"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

func (s *Service) subscribeFileChanges(ctx context.Context) {
	for e, err := range s.eventBus.SubscribeFileChanged(ctx) {
		if err != nil {
			s.logger.Error("failed to receive file changed event", zap.Error(err))
			continue
		}
		if err := s.handleFileChange(ctx, e); err != nil {
			s.logger.Error("failed to handle file changed event",
				zap.Stringer("action", e.Action),
				zap.String("relPath", e.RelPath),
				zap.Stringer("directoryID", e.DirectoryID),
				zap.Error(err))
		}
	}
}

func (s *Service) handleFileChange(ctx context.Context, e *shared.FileChangedEvent) error {
	relPath := e.RelPath
	isXMP := strings.HasSuffix(strings.ToLower(relPath), ".xmp")
	if isXMP {
		relPath = relPath[:len(relPath)-4] // 去除 .xmp 后缀以定位到图片本身
	}

	var img *image.Image
	if e.Action == shared.FileActionCreate || e.Action == shared.FileActionWrite || (e.Action == shared.FileActionRemove && isXMP) {
		var err error
		img, err = s.imageRepo.Lookup(ctx, relPath)
		if err != nil {
			if !os.IsNotExist(err) {
				return err
			}
			// 如果图片不存在，img 保持为 nil，后续会进入 RemoveImageByAbsPath 处理
		}
	}

	for sessionID, err := range s.sessionRepo.FindByDirectory(e.DirectoryID) {
		if err != nil {
			return err
		}

		sess, release, err := s.sessionRepo.Acquire(ctx, sessionID)
		if err != nil {
			s.logger.Error("failed to take ownership of session",
				zap.Stringer("sessionID", sessionID),
				zap.Error(err))
			continue
		}

		changed := false
		if img != nil {
			// 创建或更新
			filterFunc := s.imageFilterBuilder.Build(util.UnwrapPointer(sess.Filter()))
			changed = sess.UpdateImage(img, filterFunc(img))
		} else {
			// 删除，或未获取到图片的创建/更新（按删除处理）
			changed = sess.RemoveImageByAbsPath(filepath.Join(s.rootDir, relPath))
		}

		if changed {
			s.sessionSaved.Publish(ctx, sess.ID())
		}

		release()
	}

	return nil
}
