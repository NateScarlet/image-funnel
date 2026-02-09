package session

import (
	"context"
	"errors"
	"iter"
	"main/internal/apperror"
	"main/internal/domain/image"
	"main/internal/domain/metadata"
	"main/internal/shared"
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
	filter := image.BuildImageFilter(s.filter)
	return func(yield func(*image.Image, shared.ImageAction) bool) {
		for _, img := range s.images {
			if !filter(img) {
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

func (s *Service) Commit(ctx context.Context, session *Session, writeActions *shared.WriteActions) (int, error) {
	var errs []error
	var successCount int

	// 遍历所有持有且符合当前筛选条件的图片操作
	for img, action := range session.Actions() {

		var rating int
		switch action {
		case shared.ImageActionKeep:
			rating = writeActions.KeepRating
		case shared.ImageActionShelve:
			rating = writeActions.ShelveRating
		case shared.ImageActionReject:
			rating = writeActions.RejectRating
		}

		// 显式重新加载图片最新状态
		// Session 中存储的是绝对路径，而 Scanner.LookupImage 期望相对路径
		relPath, err := filepath.Rel(s.rootDir, img.Path())
		if err != nil {
			errs = append(errs, err)
			continue
		}

		currentImg, err := s.dirScanner.LookupImage(ctx, relPath)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		// 如果 ID 不匹配（说明文件已被外部修改），记录错误并跳过
		if currentImg.ID() != img.ID() {
			errs = append(errs, apperror.New(
				"IMAGE_MODIFIED_EXTERNALLY",
				"image ID mismatch (file modified externally): "+img.Path(),
				"图片 ID 不匹配（文件已被外部修改）: "+img.Path(),
			))
			continue
		}

		// 如果当前磁盘状态（即刚刚加载的状态）已经符合目标 Rating，跳过写入
		if rating == currentImg.Rating() {
			continue
		}

		xmpData := metadata.NewXMPData(rating, action.String(), time.Now())

		if err := s.metadataRepo.Write(img.Path(), xmpData); err != nil {
			errs = append(errs, err)
			continue
		}
		successCount++

		// 写入成功后，构建新的 Image 对象并直接更新内存
		// 强制使用新 Rating，但保留原图其他信息（如 ModTime，等待 FileWatcher 慢慢更新）
		newImg := image.NewImage(
			currentImg.ID(),
			currentImg.Filename(),
			currentImg.Path(),
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

	s.sessionSaved.Publish(ctx, session)

	return successCount, errors.Join(errs...)
}
