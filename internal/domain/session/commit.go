package session

import (
	"context"
	"errors"
	"iter"
	"main/internal/apperror"
	"main/internal/domain/image"
	"main/internal/domain/metadata"
	"main/internal/shared"
	"main/internal/util"
	"path/filepath"
	"time"
)

// #region Session Getter

// CanCommit 判断会话是否可以提交
//
// 提交条件：
// 1. 至少有一张图片已被处理
// 2. 或者有图片被从队列中移除
func (s *Session) CanCommit() bool {
	return s.currentIdx > 0 || s.currentRound > 0
}

func (s *Session) Actions() iter.Seq2[*image.Image, shared.ImageAction] {
	filter := s.imageFilterBuilder.Build(util.UnwrapPointer(s.filter))
	return func(yield func(*image.Image, shared.ImageAction) bool) {
		for _, img := range s.images {
			// 如果图片已被强制物理删除，则跳过
			if s.removed != nil && s.removed[img.ID()] {
				continue
			}
			// 如果图片符合 filter，或者它在本会话中已经被 commit 过，则允许提交
			if !filter(img) && !s.committed[img.ID()] {
				continue
			}
			if action, ok := s.actions[img.ID()]; ok {
				if !yield(img, action) {
					return
				}
			}
		}
	}
}

// #endregion

func (s *Service) Commit(ctx context.Context, session *Session, writeActions *shared.WriteActions) (written int, matched int, err error) {
	var errs []error

	// 遍历所有持有且符合当前筛选条件的图片操作
	for img, action := range session.Actions() {
		session.MarkCommitted(img.ID())

		var targetRating *int
		switch action {
		case shared.ImageActionKeep:
			targetRating = writeActions.KeepRating
		case shared.ImageActionShelve:
			targetRating = writeActions.ShelveRating
		case shared.ImageActionReject:
			targetRating = writeActions.RejectRating
		}

		if targetRating == nil {
			matched++
			continue
		}
		rating := *targetRating

		// 显式重新加载图片最新状态
		// Session 中存储的是相对路径
		currentImg, err := s.imageRepo.Get(ctx, img.RelPath())
		if err != nil {
			errs = append(errs, err)
			continue
		}

		// 如果 ID 不匹配（说明文件已被外部修改），记录错误并跳过
		if currentImg.ID() != img.ID() {
			errs = append(errs, apperror.New(
				"IMAGE_MODIFIED_EXTERNALLY",
				"image ID mismatch (file modified externally): "+filepath.Join(s.rootDir, img.RelPath()),
				"图片 ID 不匹配（文件已被外部修改）: "+filepath.Join(s.rootDir, img.RelPath()),
			))
			continue
		}

		// 如果当前磁盘状态（即刚刚加载的状态）已经符合目标 Rating，跳过写入
		if rating == currentImg.Rating() {
			matched++
			continue
		}

		xmpData := metadata.NewXMPData(rating, action.String(), time.Now(), currentImg.Label())

		absPath := filepath.Join(s.rootDir, img.RelPath())
		if err := s.metadataRepo.Write(absPath, xmpData); err != nil {
			errs = append(errs, err)
			continue
		}

		// 发布元数据更新事件
		s.metadataUpdatedPub.Publish(ctx, &shared.MetadataUpdatedEvent{
			ID:        currentImg.ID(),
			Path:      absPath,
			Rating:    rating,
			Label:     currentImg.Label(),
			Action:    action.String(),
			OldRating: currentImg.Rating(),
			OldLabel:  currentImg.Label(),
			OldAction: func() string {
				if currentImg.XMPData() != nil {
					return currentImg.XMPData().Action()
				}
				return ""
			}(),
		})

		written++
		matched++

		// 写入成功后，构建新的 Image 对象并直接更新内存
		// 强制使用新 Rating，但保留原图其他信息（如 ModTime，等待 FileWatcher 慢慢更新）
		newImg := image.New(
			currentImg.ID(),
			currentImg.Filename(),
			currentImg.RelPath(),
			session.DirectoryID(),
			currentImg.Size(),
			currentImg.ModTime(),
			xmpData,
			currentImg.Width(),
			currentImg.Height(),
		)

		// 直接更新内存中的图片（已持有写锁）
		if idx, ok := session.indexByID[img.ID()]; ok {
			session.images[idx] = newImg
		}
	}

	session.updatedAt = time.Now()

	s.sessionSaved.Publish(ctx, session.ID())

	dir, err := s.directoryResolver.GetDirectory(ctx, session.DirectoryID())
	if err == nil && dir != nil {
		if err := s.hookRunner.OnCommitSession(ctx, dir.RelPath()); err != nil {
			errs = append(errs, err)
		}
	}

	return written, matched, errors.Join(errs...)
}
