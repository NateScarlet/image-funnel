package session

import (
	"context"
	"main/internal/apperror"
	"main/internal/scalar"
	"main/internal/shared"
	"slices"
	"time"
)

// #region Session Methods

// MarkImage 标记指定图片的操作状态，并更新会话状态
//
// 参数：
// - imageID: 要标记的图片 ID
// - action: 要应用的操作状态
// - options: 可选参数，如操作耗时
func (s *Session) MarkImage(imageID scalar.ID, action shared.ImageAction, options ...shared.MarkImageOption) error {
	opts := shared.NewMarkImageOptions(options...)

	// 查找图片索引
	targetImgIdx, ok := s.indexByID[imageID]
	if !ok {
		return apperror.NewErrDocumentNotFound(imageID)
	}

	// 判断要标记的是否是当前图片
	isCurrentImage := s.currentIdx < len(s.queue) &&
		s.queue[s.currentIdx] == targetImgIdx

	// 检查图片是否在当前轮队列中
	inQueue := slices.Contains(s.queue, targetImgIdx)

	// 保存当前队列状态以备 Undo 恢复
	prevQueue := make([]int, len(s.queue))
	copy(prevQueue, s.queue)

	// 记录撤销操作
	prevAction, hasPrevAction := s.actions[imageID]
	var previousIndex = s.currentIdx
	s.undoStack = append(s.undoStack, func() {
		// 恢复操作状态
		if !hasPrevAction {
			delete(s.actions, imageID)
		} else {
			s.actions[imageID] = prevAction
		}
		// 注意：不恢复耗时 (durations)，因为我们需要记录用户在图片上花费的总时长（包括撤销重做的过程）

		// 恢复队列状态
		s.queue = prevQueue

		if isCurrentImage {
			// 只有当图片依然在队列中时，才恢复 currentIdx。
			// 如果图片已被物理删除并从队列中移出，恢复索引可能会导致指针错乱或越界。
			if slices.Contains(s.queue, targetImgIdx) {
				s.currentIdx = previousIndex
			}
		}
		s.updatedAt = time.Now()
	})

	// 乱序标记保留不在当前轮队列中的图片时，自动将其加入当前轮队列
	if !inQueue && action == shared.ImageActionKeep {
		s.queue = append(s.queue, targetImgIdx)
	}

	s.actions[imageID] = action
	// 累加耗时
	if !opts.Duration().IsZero() {
		s.durations[imageID] = s.durations[imageID].Add(opts.Duration())
	}
	s.updatedAt = time.Now()

	// 只有标记当前图片时才推进队列索引
	if isCurrentImage {
		s.currentIdx++
	}

	// 已在队列末尾时（含刚推进 OR 乱序标记时已越界），检查是否需要开启新一轮。
	// 乱序标记已越界的情况：用户在"刚完成"状态下回头改变某张图片为 Keep，
	// 使 Kept > targetKeep，此时必须触发 NextRound，否则 currentImage 将永远为 null。
	if err := s.tryAdvanceRound(); err != nil {
		return err
	}

	return nil
}

// #endregion

// MarkImage 标记图片并保存
func (s *Service) MarkImage(ctx context.Context, sessionID scalar.ID, imageID scalar.ID, action shared.ImageAction, options ...shared.MarkImageOption) error {
	sess, release, err := s.sessionRepo.Acquire(ctx, sessionID)
	if err != nil {
		return err
	}
	defer release()

	if err := sess.MarkImage(imageID, action, options...); err != nil {
		return err
	}

	s.sessionSaved.Publish(ctx, sess.ID())
	return nil
}
