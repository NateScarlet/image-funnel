package session

import (
	"context"
	"main/internal/domain/directory"
	"main/internal/domain/image"
	"main/internal/scalar"
	"main/internal/shared"
	"sort"
	"time"
)

// #region Update Options

// UpdateOptions 定义会话更新选项
type UpdateOptions struct {
	targetKeep *int
	filter     *shared.ImageFilters
}

// UpdateOption 定义更新选项的函数类型
type UpdateOption func(*UpdateOptions)

// WithTargetKeep 设置目标保留数量
func WithTargetKeep(targetKeep int) UpdateOption {
	return func(opts *UpdateOptions) {
		opts.targetKeep = &targetKeep
	}
}

// WithFilter 设置过滤器
func WithFilter(filter *shared.ImageFilters) UpdateOption {
	return func(opts *UpdateOptions) {
		opts.filter = filter
	}
}

// #endregion

// #region Session Methods

// UpdateTargetKeep 更新会话的目标保留数量
func (s *Session) UpdateTargetKeep(targetKeep int) error {
	s.targetKeep = targetKeep
	s.updatedAt = time.Now()
	return nil
}

// NextRound 开启新一轮筛选
// 参数：
// - filter: 图片过滤器
// - filteredImages: 新的筛选后图片队列
func (s *Session) NextRound(filter *shared.ImageFilters, filteredImages []*image.Image) error {
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
		lastImage = s.images[prevQueue[prevIdx]]
	} else if len(prevQueue) > 0 {
		lastImage = s.images[prevQueue[len(prevQueue)-1]]
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

	// 转换 filteredImages 到 indices 并更新 images
	newQueue := make([]int, len(filteredImages))
	for i, img := range filteredImages {
		if idx, ok := s.indexByID[img.ID()]; ok {
			s.images[idx] = img // 更新引用
			newQueue[i] = idx
		} else {
			// 新增
			newIdx := len(s.images)
			s.images = append(s.images, img)
			s.indexByID[img.ID()] = newIdx
			s.indexByPath[img.Path()] = newIdx
			newQueue[i] = newIdx
		}
	}
	s.queue = newQueue

	s.currentIdx = 0
	s.updatedAt = time.Now()

	return nil
}

// #endregion

// Update 更新会话配置
// 使用 Options 模式支持灵活的更新选项
func (s *Service) Update(ctx context.Context, id scalar.ID, options ...UpdateOption) error {
	sess, release, err := s.sessionRepo.Acquire(ctx, id)
	if err != nil {
		return err
	}
	defer release()

	opts := &UpdateOptions{}
	for _, opt := range options {
		opt(opts)
	}

	if opts.targetKeep != nil {
		if err := sess.UpdateTargetKeep(*opts.targetKeep); err != nil {
			return err
		}
	}

	if opts.filter != nil {
		directory, err := directory.DecodeID(sess.DirectoryID())
		if err != nil {
			return err
		}

		filterFunc := image.BuildImageFilter(opts.filter)
		var filteredImages []*image.Image
		for img, err := range s.dirScanner.Scan(ctx, directory) {
			if err != nil {
				return err
			}
			if filterFunc(img) {
				filteredImages = append(filteredImages, img)
			}
		}

		if err := sess.NextRound(opts.filter, filteredImages); err != nil {
			return err
		}
	}

	s.sessionSaved.Publish(ctx, sess)
	return nil
}
