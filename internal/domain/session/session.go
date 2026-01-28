package session

import (
	"iter"
	"main/internal/apperror"
	"main/internal/domain/image"
	"main/internal/scalar"
	"main/internal/shared"
	"sort"
	"sync"
	"time"
)

// WriteActions 定义了不同操作对应的评分值
// 用于将图片操作映射到 XMP 评分系统

type WriteActions struct {
	keepRating   int
	shelveRating int
	rejectRating int
}

func NewWriteActions(keepRating, shelveRating, rejectRating int) *WriteActions {
	return &WriteActions{
		keepRating:   keepRating,
		shelveRating: shelveRating,
		rejectRating: rejectRating,
	}
}

func (a *WriteActions) KeepRating() int {
	return a.keepRating
}

func (a *WriteActions) ShelveRating() int {
	return a.shelveRating
}

func (a *WriteActions) RejectRating() int {
	return a.rejectRating
}

// Stats 表示会话的统计信息，用于跟踪筛选进度和结果

type Stats struct {
	total       int
	kept        int
	shelved     int
	rejected    int
	remaining   int
	targetKeep  int
	isCompleted bool
}

func (s *Stats) Total() int {
	return s.total
}

func (s *Stats) Kept() int {
	return s.kept
}

func (s *Stats) Shelved() int {
	return s.shelved
}

func (s *Stats) Rejected() int {
	return s.rejected
}

func (s *Stats) Remaining() int {
	return s.remaining
}

func (s *Stats) TargetKeep() int {
	return s.targetKeep
}

func (s *Stats) IsCompleted() bool {
	return s.isCompleted
}

// Session 表示一个图片筛选会话，包含筛选过程中的所有状态和操作
//
// 会话流程：
// 1. 初始化时创建包含所有图片的队列
// 2. 用户对图片进行评分（保留/搁置/排除）
// 3. 当队列处理完成后，根据评分重新组织队列进行下一轮筛选
// 4. 直到达到目标保留数量或所有图片都被处理
// 5. 提交会话结果，将评分写入 XMP Sidecar 文件

type Session struct {
	id          scalar.ID            // 会话唯一标识符
	directoryID scalar.ID            // 目录 ID
	filter      *shared.ImageFilters // 图片过滤器，用于筛选特定类型的图片
	targetKeep  int                  // 目标保留图片数量
	createdAt   time.Time            // 会话创建时间
	updatedAt   time.Time            // 会话最后更新时间

	queue      []*image.Image                   // 当前待处理的图片队列
	images     map[scalar.ID]*image.Image       // 会话中所有历史图片
	currentIdx int                              // 当前处理的图片在队列中的索引
	undoStack  []UndoEntry                      // 撤销操作栈
	actions    map[scalar.ID]shared.ImageAction // 图片操作映射

	roundHistory []RoundSnapshot // 轮次历史记录
	currentRound int             // 当前筛选轮次

	mu sync.RWMutex // 读写互斥锁，用于并发安全访问
}

// RoundSnapshot 表示一轮筛选的快照，用于存储筛选轮次的状态
// 当需要撤销到上一轮时，使用此快照恢复状态

type RoundSnapshot struct {
	queue      []*image.Image
	undoStack  []UndoEntry
	currentIdx int
	filter     *shared.ImageFilters
}

// NewSession 创建一个新的图片筛选会话
//
// 参数：
// - id: 会话唯一标识符
// - directoryID: 目录 ID
// - filter: 图片过滤器
// - targetKeep: 目标保留图片数量
// - images: 待处理的图片集合
func NewSession(id scalar.ID, directoryID scalar.ID, filter *shared.ImageFilters, targetKeep int, images []*image.Image) *Session {
	actions := make(map[scalar.ID]shared.ImageAction)
	imagesMap := make(map[scalar.ID]*image.Image)
	for _, img := range images {
		imagesMap[img.ID()] = img
	}
	return &Session{
		id:           id,
		directoryID:  directoryID,
		filter:       filter,
		targetKeep:   targetKeep,
		createdAt:    time.Now(),
		updatedAt:    time.Now(),
		queue:        images,
		images:       imagesMap,
		currentIdx:   0,
		undoStack:    make([]UndoEntry, 0),
		actions:      actions,
		roundHistory: make([]RoundSnapshot, 0),
		currentRound: 0,
	}
}

func (s *Session) ID() scalar.ID {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.id
}

func (s *Session) DirectoryID() scalar.ID {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.directoryID
}

func (s *Session) Filter() *shared.ImageFilters {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.filter
}

func (s *Session) TargetKeep() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.targetKeep
}

func (s *Session) CreatedAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.createdAt
}

func (s *Session) UpdatedAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.updatedAt
}

func (s *Session) CurrentImage() *image.Image {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.currentIdx < len(s.queue) {
		return s.queue[s.currentIdx]
	}
	return nil
}

func (s *Session) NextImage() *image.Image {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.currentIdx+1 < len(s.queue) {
		return s.queue[s.currentIdx+1]
	}
	return nil
}

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

func (s *Session) CurrentIndex() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentIdx
}

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

// setFilter 更新会话的图片过滤器
func (s *Session) setFilter(filter *shared.ImageFilters) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.filter = filter
	s.updatedAt = time.Now()
	return nil
}

// nextRound 开启新一轮筛选
// 参数：
// - filter: 图片过滤器
// - filteredImages: 新的筛选后图片队列
func (s *Session) nextRound(filter *shared.ImageFilters, filteredImages []*image.Image) error {
	// 检查是否已经获取了锁
	// 由于 MarkImage 函数已经获取了锁，这里需要避免重复获取
	// 直接执行逻辑，不获取锁

	// 保存当前状态到历史记录
	if len(s.queue) > 0 {
		s.roundHistory = append(s.roundHistory, RoundSnapshot{
			queue:      s.queue,
			undoStack:  s.undoStack,
			currentIdx: s.currentIdx,
			filter:     s.filter,
		})
	}

	// 开启新一轮
	s.currentRound++
	if filter != nil {
		s.filter = filter
	}
	s.queue = filteredImages
	s.currentIdx = 0
	s.undoStack = make([]UndoEntry, 0)
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

func (s *Session) Stats() *Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats()
}

// stats 计算会话的统计信息，包括处理进度和各种操作的图片数量
//
// 注意：此方法是内部方法，不进行并发控制，调用者需要自行保证线程安全
func (s *Session) stats() *Stats {
	var stats Stats
	stats.total = len(s.queue)
	stats.remaining = len(s.queue) - s.currentIdx
	stats.targetKeep = s.targetKeep

	for i := 0; i < s.currentIdx; i++ {
		img := s.queue[i]
		action := s.actions[img.ID()]
		switch action {
		case shared.ImageActionKeep:
			stats.kept++
		case shared.ImageActionShelve:
			stats.shelved++
		case shared.ImageActionReject:
			stats.rejected++
		}
	}

	// 计算被 "丢弃" 的图片数量（即曾经有过操作记录，但在当前队列中不存在的图片）
	// 这通常发生在 session 重新组织（NextRound）时，未保留的图片被移出队列
	// 或者 AddImage 添加的新图片尚未有操作记录（不计入丢弃）

	// 使用 images map 记录了会话中所有历史图片
	// queue 记录了当前轮次正在进行的图片（包含已处理和未处理的）
	// 因此，rejected (丢弃/排除) = 总数 - 当前队列长度
	stats.rejected += len(s.images) - len(s.queue)

	// 计算isCompleted字段
	// 会话完成条件：
	// 1. 所有图片都已处理 (remaining == 0)
	// 2. 且保留的图片数量不超过目标保留数量 (否则需要开启新一轮)
	// 注意：搁置 (Shelve) 的图片不计入目标保留数量计算，因为它们在本会话中被视为已丢弃
	stats.isCompleted = stats.remaining == 0 && (stats.kept <= stats.targetKeep)

	return &stats
}

// CanCommit 判断会话是否可以提交
//
// 提交条件：
// 1. 至少有一张图片已被处理
// 2. 或者有图片被从队列中移除
func (s *Session) CanCommit() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.currentIdx > 0 || len(s.roundHistory) > 0
}

// CanUndo 判断会话是否可以执行撤销操作
//
// 撤销条件：
// 1. 撤销栈不为空，或
// 2. 存在历史轮次（支持跨轮撤销）
func (s *Session) CanUndo() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.undoStack) > 0 || len(s.roundHistory) > 0
}

// MarkImage 标记指定图片的操作状态，并更新会话状态
//
// 参数：
// - imageID: 要标记的图片 ID
// - action: 要应用的操作状态
func (s *Session) MarkImage(imageID scalar.ID, action shared.ImageAction) error {
	s.mu.Lock()
	defer s.mu.Unlock()

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

	s.undoStack = append(s.undoStack, UndoEntry{
		imageID: imageID,
		action:  s.actions[imageID],
		index:   s.currentIdx,
	})

	s.actions[imageID] = action
	s.updatedAt = time.Now()

	s.currentIdx++

	if s.currentIdx >= len(s.queue) {
		stats := s.stats()

		if stats.kept > 0 {
			var newQueue []*image.Image
			for _, img := range s.queue {
				action := s.actions[img.ID()]
				if action == shared.ImageActionKeep {
					newQueue = append(newQueue, img)
				}
			}

			if len(newQueue) > 0 {
				if len(newQueue) > s.targetKeep {
					// 开启新一轮
					if err := s.nextRound(nil, newQueue); err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

// addFilteredImage 添加新图片到会话
// 如果图片符合过滤器且不在队列中，则添加
func (s *Session) addFilteredImage(img *image.Image) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addFilteredImageLocked(img)
}

func (s *Session) addFilteredImageLocked(img *image.Image) error {
	// 检查是否已经在队列中
	if _, existing := s.images[img.ID()]; existing {
		return nil
	}

	s.queue = append(s.queue, img)
	s.images[img.ID()] = img
	// 注意：不设置 s.actions[img.ID()]，因为 actions 仅存储用户显式操作
	// 默认无操作记录，表示尚未处理
	s.updatedAt = time.Now()

	// 如果会话已完成，添加新图片后可能变为未完成
	// stats() 会自动计算

	return nil
}

// Undo 撤销上一次图片标记操作，恢复到之前的状态

func (s *Session) Undo() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.undoStack) == 0 {
		// 跨轮次撤销
		if len(s.roundHistory) == 0 {
			return ErrNothingToUndo
		}

		lastRound := s.roundHistory[len(s.roundHistory)-1]
		s.roundHistory = s.roundHistory[:len(s.roundHistory)-1]
		s.currentRound--
		s.queue = lastRound.queue
		s.undoStack = lastRound.undoStack
		s.currentIdx = lastRound.currentIdx
		s.filter = lastRound.filter

		// 如果恢复到了队列末尾（意味着这是正常处理完一轮触发的），
		// 且 undoStack 不为空，则自动再撤销一步，以提供更好的用户体验（直接撤销导致换轮的那个操作）。
		// 否则（例如 UpdateSession 导致的换轮，此时还在队列中间），只恢复状态，不撤销操作。
		if s.currentIdx < len(s.queue) || len(s.undoStack) == 0 {
			return nil
		}
	}

	// 普通撤销
	lastEntry := s.undoStack[len(s.undoStack)-1]
	s.undoStack = s.undoStack[:len(s.undoStack)-1]

	if lastEntry.action.IsZero() {
		delete(s.actions, lastEntry.imageID)
	} else {
		s.actions[lastEntry.imageID] = lastEntry.action
	}

	s.currentIdx = lastEntry.index
	s.updatedAt = time.Now()
	return nil
}

// #region 维护方法

// UpdateImageByPath 根据路径更新图片信息
func (s *Session) UpdateImageByPath(img *image.Image, matchesFilter bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	var oldID scalar.ID
	var oldIndex = -1
	for id, existing := range s.images {
		if existing.Path() == img.Path() {
			oldID = id
			break
		}
	}

	for i, existing := range s.queue {
		if existing.Path() == img.Path() {
			oldIndex = i
			break
		}
	}

	// 如果原本不在会话中
	if oldID.IsZero() && oldIndex == -1 {
		if matchesFilter {
			// 如果匹配过滤器，执行添加逻辑
			s.addFilteredImageLocked(img)
			return true
		}
		return false
	}

	// 如果不匹配过滤器，从会话中移除
	if !matchesFilter {
		return s.removeImageByPathLocked(img.Path())
	}

	// 更新图片信息
	if !oldID.IsZero() {
		delete(s.images, oldID)
		s.images[img.ID()] = img

		// 如果 ID 发生了变化（通常是由于修改时间变化）
		if oldID != img.ID() {
			if action, ok := s.actions[oldID]; ok {
				s.actions[img.ID()] = action
				delete(s.actions, oldID)
			}
			// 更新撤销栈中的图片 ID
			for i := range s.undoStack {
				if s.undoStack[i].imageID == oldID {
					s.undoStack[i].imageID = img.ID()
				}
			}
		}
	}

	if oldIndex != -1 {
		s.queue[oldIndex] = img
	}

	s.updatedAt = time.Now()
	return true
}

// RemoveImageByPath 根据路径从会话中移除图片
func (s *Session) RemoveImageByPath(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.removeImageByPathLocked(path)
}

func (s *Session) removeImageByPathLocked(path string) bool {
	var targetID scalar.ID
	var targetIndex = -1
	for id, img := range s.images {
		if img.Path() == path {
			targetID = id
			break
		}
	}

	if targetID.IsZero() {
		return false
	}

	for i, img := range s.queue {
		if img.ID() == targetID {
			targetIndex = i
			break
		}
	}

	delete(s.images, targetID)
	delete(s.actions, targetID)

	if targetIndex != -1 {
		s.queue = append(s.queue[:targetIndex], s.queue[targetIndex+1:]...)
		if targetIndex < s.currentIdx {
			s.currentIdx--
		}

		// 修正撤销栈
		newUndoStack := make([]UndoEntry, 0, len(s.undoStack))
		for _, entry := range s.undoStack {
			if entry.imageID == targetID {
				continue
			}
			// 如果移除的是当前索引之前的图片，需要调整撤销栈中的索引
			if entry.index > targetIndex {
				entry.index--
			}
			newUndoStack = append(newUndoStack, entry)
		}
		s.undoStack = newUndoStack
	}

	s.updatedAt = time.Now()
	return true
}

// #endregion

func (s *Session) Actions() iter.Seq2[*image.Image, shared.ImageAction] {
	return func(yield func(*image.Image, shared.ImageAction) bool) {
		s.mu.RLock()
		defer s.mu.RUnlock()

		for _, img := range s.images {
			if action, ok := s.actions[img.ID()]; ok {
				if !yield(img, action) {
					return
				}
			}
		}
	}
}

// UndoEntry 表示一个可撤销的操作条目，用于存储图片操作的历史状态
// 当执行撤销操作时，使用此条目恢复图片的原始操作状态
type UndoEntry struct {
	imageID scalar.ID
	action  shared.ImageAction
	index   int
}

var (
	ErrNoMoreImages  = apperror.New("INVALID_OPERATION", "no more images", "没有更多图片")
	ErrNothingToUndo = apperror.New("INVALID_OPERATION", "nothing to undo", "没有可以撤销的操作")
)
