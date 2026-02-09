package session

import (
	"context"
	"main/internal/domain/image"
	"main/internal/shared"
	"os"
	"path/filepath"

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
	var img *image.Image
	if e.Action == shared.FileActionCreate || e.Action == shared.FileActionWrite {
		var err error
		img, err = s.dirScanner.LookupImage(ctx, e.RelPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
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
			filterFunc := image.BuildImageFilter(sess.Filter())
			changed = sess.UpdateImage(img, filterFunc(img))
		} else {
			// 删除，或未获取到图片的创建/更新（按删除处理）
			changed = sess.RemoveImageByPath(filepath.Join(s.rootDir, e.RelPath))
		}

		if changed {
			s.sessionSaved.Publish(ctx, sess)
		}

		release()
	}

	return nil
}
