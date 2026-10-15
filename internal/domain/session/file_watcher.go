package session

import (
	"context"
	"main/internal/domain/directory"
	"main/internal/domain/image"
	"main/internal/shared"
	"main/internal/util"
	"os"

	"go.uber.org/zap"
)

func (s *Service) subscribeFileChanges(ctx context.Context) {
	for e, err := range s.fileChangedSub.Subscribe(ctx) {
		if err != nil {
			s.logger.Error("failed to receive file changed event", zap.Error(err))
			continue
		}
		if err := s.handleFileChange(ctx, e); err != nil {
			s.logger.Error("failed to handle file changed event",
				zap.Stringer("action", e.Action),
				zap.String("relPath", e.RelPath.String()),
				zap.Stringer("directoryID", e.DirectoryID),
				zap.Error(err))
		}
	}
}

func (s *Service) handleFileChange(ctx context.Context, e *directory.FileChangedEvent) error {
	// XMP 是图片的配套文件，事件路径去掉后缀后即图片本体路径；
	// 不是配套文件时原样返回事件路径。判定与裁剪同源于 TrimExtSuffix，
	// 避免大小写不一致的后缀漏裁
	relPath, isXMP, err := e.RelPath.TrimExtSuffix(s.rootDir, ".xmp")
	if err != nil {
		return err
	}

	var img *image.Image
	if e.Action == shared.FileActionCreate || e.Action == shared.FileActionWrite || (e.Action == shared.FileActionRemove && isXMP) {
		img, err = s.imageRepo.Get(ctx, relPath)
		if err != nil {
			if !os.IsNotExist(err) {
				return err
			}
			// 如果图片不存在，img 保持为 nil，后续会进入 RemoveImageByRelPath 处理
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
			var err error
			changed, err = sess.UpdateImage(img, filterFunc(img))
			if err != nil {
				release()
				return err
			}
		} else {
			// 删除，或未获取到图片的创建/更新（按删除处理）
			var err error
			changed, err = sess.removeImageByRelPath(relPath, true)
			if err != nil {
				release()
				return err
			}
		}

		if changed {
			s.sessionSaved.Publish(ctx, sess.ID())
		}

		release()
	}

	return nil
}
