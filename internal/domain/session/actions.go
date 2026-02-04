package session

import (
	"main/internal/apperror"
	"main/internal/domain/image"
	"main/internal/scalar"
	"main/internal/shared"
	"sort"
	"time"
)

// CurrentImage 返回当前正在处理的图片
func (s *Session) CurrentImage() *image.Image {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.currentIdx < len(s.queue) {
		return s.queue[s.currentIdx]
	}
	return nil
}

// NextImages 返回指定数量的后续图片
func (s *Session) NextImages(count int) []*image.Image {
	if count == 0 {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if count < 0 {
		// 返回所有
		return s.queue[s.currentIdx+1:]
	}
	start := s.currentIdx + 1
	if start >= len(s.queue) {
		return nil
	}
	end := start + count
	if end > len(s.queue) {
		end = len(s.queue)
	}
	return s.queue[start:end]
}

// KeptImages 返回所有已被标记为保留的图片
func (s *Session) KeptImages(limit, offset int) []*image.Image {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var kept []*image.Image
	for id, img := range s.images {
		if s.actions[id] == shared.ImageActionKeep {
			kept = append(kept, img)
		}
	}

	// 按文件名排序，确保分页确定性
	sort.Slice(kept, func(i, j int) bool {
		return kept[i].Filename() < kept[j].Filename()
	})

	if offset >= len(kept) {
		return nil
	}

	end := offset + limit
	if limit < 0 {
		end = len(kept)
	} else if end > len(kept) {
		end = len(kept)
	}

	return kept[offset:end]
}

// CurrentIndex 返回当前处理图片的索引
func (s *Session) CurrentIndex() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentIdx
}

// CurrentSize 返回当前队列的总图片数量
func (s *Session) CurrentSize() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.queue)
}

// UpdateTargetKeep 更新会话的目标保留数量
func (s *Session) UpdateTargetKeep(targetKeep int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.targetKeep = targetKeep
	s.updatedAt = time.Now()

	return nil
}

// nextRound 开启新一轮筛选
// 参数：
// - filter: 图片过滤器
// - filteredImages: 新的筛选后图片队列
func (s *Session) nextRound(filter *shared.ImageFilters, filteredImages []*image.Image) error {

	// 保存当前状态到撤销栈，以便撤销换轮操作
	prevQueue := s.queue
	prevFilter := s.filter
	prevRound := s.currentRound
	prevIdx := s.currentIdx

	// 按照操作耗时排序，耗时短的排在前面
	// 如果耗时相同，保持原有的相对顺序（sort.SliceStable）
	sort.SliceStable(filteredImages, func(i, j int) bool {
		return s.durations[filteredImages[i].ID()].Nanoseconds() < s.durations[filteredImages[j].ID()].Nanoseconds()
	})

	// 避免连续出现同一张图片
	// 如果排序后的第一张是上一轮正在看或最后看的那一张，则将它放到第二张
	var lastImage *image.Image
	if prevIdx < len(prevQueue) {
		lastImage = prevQueue[prevIdx]
	} else if len(prevQueue) > 0 {
		lastImage = prevQueue[len(prevQueue)-1]
	}
	if lastImage != nil && len(filteredImages) > 1 && filteredImages[0].ID() == lastImage.ID() {
		filteredImages[0], filteredImages[1] = filteredImages[1], filteredImages[0]
	}

	s.undoStack = append(s.undoStack, func() {
		s.queue = prevQueue
		s.filter = prevFilter
		s.currentRound = prevRound
		s.currentIdx = prevIdx
		s.updatedAt = time.Now()

		// 检查是否刚刚恢复到了某一轮的末尾 (currentIdx >= len)
		// 这意味着上一轮已经完成，用户可能希望继续撤销导致完成的那个操作，
		// 以便直接回到上一轮的最后一张图片进行修改。
		// 如果不这样做，用户会面对一个“已完成”的界面，必须再次操作才能看到图片。
		if s.currentIdx >= len(s.queue) && len(s.undoStack) > 0 {
			nextFunc := s.undoStack[len(s.undoStack)-1]
			s.undoStack = s.undoStack[:len(s.undoStack)-1]
			nextFunc()
		}
	})

	// 开启新一轮
	s.currentRound++
	if filter != nil {
		s.filter = filter
	}
	s.queue = filteredImages
	// 确保所有在队列中的图片都在 images 映射中
	// 这对于 Service.Update 触发的 NextRound 特别重要，因为它引入了新的图片对象
	// 如果不这样做，Commit() 遍历 s.images 时会遗漏这些图片，导致标记操作无法写入
	for _, img := range filteredImages {
		s.images[img.ID()] = img
	}

	s.currentIdx = 0
	s.updatedAt = time.Now()

	return nil
}

// NextRound 开启新一轮筛选（带锁版本）
// 用于外部直接调用，会自动获取和释放锁
func (s *Session) NextRound(filter *shared.ImageFilters, filteredImages []*image.Image) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nextRound(filter, filteredImages)
}

// CanCommit 判断会话是否可以提交
//
// 提交条件：
// 1. 至少有一张图片已被处理
// 2. 或者有图片被从队列中移除
func (s *Session) CanCommit() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.currentIdx > 0 || s.currentRound > 0
}

// CanUndo 判断会话是否可以执行撤销操作
//
// 撤销条件：
// 1. 撤销栈不为空，或
// 2. 存在历史轮次（支持跨轮撤销）
func (s *Session) CanUndo() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.undoStack) > 0
}

// MarkImage 标记指定图片的操作状态，并更新会话状态
//
// 参数：
// - imageID: 要标记的图片 ID
// - action: 要应用的操作状态
// - options: 可选参数，如操作耗时
func (s *Session) MarkImage(imageID scalar.ID, action shared.ImageAction, options ...shared.MarkImageOption) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	opts := shared.NewMarkImageOptions(options...)

	if s.currentIdx >= len(s.queue) {
		return ErrNoMoreImages
	}

	currentImage := s.queue[s.currentIdx]
	if currentImage.ID() != imageID {
		found := false
		for i, img := range s.queue {
			if img.ID() == imageID {
				currentImage = img
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
		stats := s.stats()

		if stats.kept > s.targetKeep {
			var newQueue []*image.Image
			for _, img := range s.queue {
				action := s.actions[img.ID()]
				if action == shared.ImageActionKeep {
					newQueue = append(newQueue, img)
				}
			}

			// 开启新一轮
			if err := s.nextRound(nil, newQueue); err != nil {
				return err
			}
		}
	}

	return nil
}

// Undo 撤销上一次图片标记操作，恢复到之前的状态
func (s *Session) Undo() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.undoStack) == 0 {
		return ErrNothingToUndo
	}

	// 执行撤销函数
	lastFunc := s.undoStack[len(s.undoStack)-1]
	s.undoStack = s.undoStack[:len(s.undoStack)-1]
	lastFunc()

	return nil
}
