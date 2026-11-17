package session

import (
	"context"
	"main/internal/apperror"
	"main/internal/domain/image"
	"main/internal/scalar"
	"main/internal/shared"
	"main/internal/util"
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
	// 内部先拷贝一份：排序与防连续对调只作用于会话内部队列，不改动调用者传入的切片
	members := make([]*image.Image, len(filteredImages))
	copy(members, filteredImages)

	// 保存当前状态到撤销栈，以便撤销换轮操作
	prevQueue := s.queue
	prevFilter := s.filter
	prevRound := s.currentRound
	prevIdx := s.currentIdx

	// 按照操作耗时排序，耗时短的排在前面
	// 如果耗时相同，保持原有的相对顺序（sort.SliceStable）
	sort.SliceStable(members, func(i, j int) bool {
		return s.durations[members[i].ID()].Nanoseconds() < s.durations[members[j].ID()].Nanoseconds()
	})

	// 避免连续出现同一张图片
	// 如果排序后的第一张是上一轮正在看或最后看的那一张，则将它放到第二张
	var lastImage *image.Image
	if prevIdx < len(prevQueue) {
		lastImage = s.images[prevQueue[prevIdx]]
	} else if len(prevQueue) > 0 {
		lastImage = s.images[prevQueue[len(prevQueue)-1]]
	}
	if lastImage != nil && len(members) > 1 && members[0].ID() == lastImage.ID() {
		members[0], members[1] = members[1], members[0]
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

	// 转换 members 到 indices 并更新 images
	newQueue := make([]int, len(members))
	for i, img := range members {
		if idx, ok := s.indexByID[img.ID()]; ok {
			s.images[idx] = img // 更新引用
			newQueue[i] = idx
		} else {
			// 新增
			newIdx := len(s.images)
			s.images = append(s.images, img)
			s.indexByID[img.ID()] = newIdx
			s.indexByRelPath[img.RelPath()] = newIdx
			newQueue[i] = newIdx
		}
	}
	s.queue = newQueue

	s.currentIdx = 0
	s.updatedAt = time.Now()

	return nil
}

// tryAdvanceRound 在队列已处理完且 Keep 数超过 targetKeep 时自动推进到下一轮。
// 消除 MarkImage 与 removeImageByRelPath 之间的重复逻辑。
func (s *Session) tryAdvanceRound() error {
	if !s.currentRoundDone() {
		return nil
	}
	stats := s.Stats()
	if stats.TotalKept <= s.targetKeep {
		return nil
	}

	// 自动推进的候选集是当前轮 Keep 图片；这些图片已进入会话队列，本身即已满足筛选条件，
	// 因此传 nil 表示不再附加筛选，保持「Keep 图片全部进入下一轮」的既有行为。
	var keepCandidates []*image.Image
	for _, idx := range s.queue {
		img := s.images[idx]
		if s.actions[img.ID()] == shared.ImageActionKeep {
			keepCandidates = append(keepCandidates, img)
		}
	}
	members := s.nextRoundMembers(keepCandidates, nil)
	return s.NextRound(nil, members)
}

// #endregion

// nextRoundMembers 是「下一轮成员」的唯一判定原语：
// 从候选图片集合中挑出属于下一轮的图片，保持候选顺序（排序由 NextRound 负责）。
// 自动推进与改筛选都经此原语，区别仅在候选集与筛选条件。
//
// 每次判定都会重建过滤闭包；标记路径的自动推进还会叠加 Stats() 的构建，
// 评估后该成本可接受，不为此引入缓存等额外复杂度。
func (s *Session) nextRoundMembers(candidates []*image.Image, filter *shared.ImageFilters) []*image.Image {
	filterFunc := s.imageFilterBuilder.Build(util.UnwrapPointer(filter))
	members := make([]*image.Image, 0, len(candidates))
	for _, img := range candidates {
		// 已被物理移除的图片不属于任何一轮
		if s.removed != nil && s.removed[img.ID()] {
			continue
		}
		if filterFunc(img) {
			members = append(members, img)
		}
	}
	return members
}

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
		// 读盘仅用于刷新会话已知图片的元数据（XMP 评分、标签等），
		// 「谁属于下一轮」交给聚合上的唯一原语判定，这里不再自行过滤。
		knownImages := sess.knownImages()
		for i, img := range knownImages {
			fresh, err := s.imageRepo.Get(ctx, img.RelPath())
			if apperror.IsNotFound(err) {
				// 文件已不在磁盘上，文件变更流程会将其标记为已移除
				continue
			}
			if err != nil {
				return err
			}
			// matchesFilter 传 true：本次只刷新引用，不借机调整队列；成员判定由原语负责
			if _, err := sess.UpdateImage(fresh, true); err != nil {
				return err
			}
			// 候选集也换成刷新后的引用，后续按最新元数据判定成员
			knownImages[i] = fresh
		}

		members := sess.nextRoundMembers(knownImages, opts.filter)
		if err := sess.NextRound(opts.filter, members); err != nil {
			return err
		}
	}

	s.sessionSaved.Publish(ctx, sess.ID())
	return nil
}
