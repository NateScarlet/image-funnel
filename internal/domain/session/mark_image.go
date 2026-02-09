package session

import (
	"context"
	"main/internal/apperror"
	"main/internal/domain/image"
	"main/internal/scalar"
	"main/internal/shared"
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

	if s.currentIdx >= len(s.queue) {
		return ErrNoMoreImages
	}

	currentImage := s.images[s.queue[s.currentIdx]]
	if currentImage.ID() != imageID {
		found := false
		for i, idx := range s.queue {
			if s.images[idx].ID() == imageID {
				currentImage = s.images[idx]
				s.currentIdx = i
				found = true
				break
			}
		}
		if !found {
			return apperror.NewErrDocumentNotFound(imageID)
		}
	}

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

		// 恢复当前索引
		s.currentIdx = previousIndex
		s.updatedAt = time.Now()
	})

	s.actions[imageID] = action
	// 累加耗时
	if !opts.Duration().IsZero() {
		s.durations[imageID] = s.durations[imageID].Add(opts.Duration())
	}
	s.updatedAt = time.Now()

	s.currentIdx++

	if s.currentIdx >= len(s.queue) {
		stats := s.Stats()

		if stats.Kept > s.targetKeep {
			var newQueue []*image.Image
			for _, idx := range s.queue {
				img := s.images[idx]
				action := s.actions[img.ID()]
				if action == shared.ImageActionKeep {
					newQueue = append(newQueue, img)
				}
			}

			// 开启新一轮
			if err := s.NextRound(nil, newQueue); err != nil {
				return err
			}
		}
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

	s.sessionSaved.Publish(ctx, sess)
	return nil
}
