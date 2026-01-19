package session

import (
	"main/internal/domain/image"
	"main/internal/scalar"
	"main/internal/shared"
	"sync"
	"time"
)

// WriteActions 定义了不同操作对应的评分值
// 用于将图片操作映射到 XMP 评分系统

type WriteActions struct {
	keepRating    int
	pendingRating int
	rejectRating  int
}

func NewWriteActions(keepRating, pendingRating, rejectRating int) *WriteActions {
	return &WriteActions{
		keepRating:    keepRating,
		pendingRating: pendingRating,
		rejectRating:  rejectRating,
	}
}

func (a *WriteActions) KeepRating() int {
	return a.keepRating
}

func (a *WriteActions) PendingRating() int {
	return a.pendingRating
}

func (a *WriteActions) RejectRating() int {
	return a.rejectRating
}

// Stats 表示会话的统计信息，用于跟踪筛选进度和结果

type Stats struct {
	total      int
	processed  int
	kept       int
	reviewed   int
	rejected   int
	remaining  int
	targetKeep int
}

func (s *Stats) Total() int {
	return s.total
}

func (s *Stats) Processed() int {
	return s.processed
}

func (s *Stats) Kept() int {
	return s.kept
}

func (s *Stats) Reviewed() int {
	return s.reviewed
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

// Status 根据统计数据计算会话状态
func (s *Stats) Status() shared.SessionStatus {
	// 如果还有剩余图片，状态为 Active
	if s.remaining > 0 {
		return shared.SessionStatusActive
	}

	// 如果没有剩余图片，根据处理结果判断
	if s.kept > 0 || s.reviewed > 0 {
		// 计算新队列长度（保留和稍后再看的图片）
		newQueueLength := s.kept + s.reviewed
		if newQueueLength <= s.targetKeep {
			// 新队列长度小于等于目标保留数量，会话完成
			return shared.SessionStatusCompleted
		} else {
			// 新队列长度大于目标保留数量，会开启新一轮筛选，状态为 Active
			return shared.SessionStatusActive
		}
	}
	// 所有图片都被标记为排除，会话完成
	return shared.SessionStatusCompleted

}

// Session 表示一个图片筛选会话，包含筛选过程中的所有状态和操作
//
// 会话流程：
// 1. 初始化时创建包含所有图片的队列
// 2. 用户对图片进行评分（保留/稍后再看/排除）
// 3. 当队列处理完成后，根据评分重新组织队列进行下一轮筛选
// 4. 直到达到目标保留数量或所有图片都被处理
// 5. 提交会话结果，将评分写入 XMP Sidecar 文件

type Session struct {
	id         scalar.ID            // 会话唯一标识符
	directory  string               // 处理的图片目录路径
	filter     *shared.ImageFilters // 图片过滤器，用于筛选特定类型的图片
	targetKeep int                  // 目标保留图片数量
	createdAt  time.Time            // 会话创建时间
	updatedAt  time.Time            // 会话最后更新时间

	queue      []*image.Image                   // 当前待处理的图片队列
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
	currentIdx int
	undoStack  []UndoEntry
}

// NewSession 创建一个新的图片筛选会话
//
// 参数：
// - id: 会话唯一标识符
// - directory: 处理的图片目录路径
// - filter: 图片过滤器
// - targetKeep: 目标保留图片数量
// - images: 待处理的图片集合
func NewSession(id scalar.ID, directory string, filter *shared.ImageFilters, targetKeep int, images []*image.Image) *Session {
	actions := make(map[scalar.ID]shared.ImageAction)
	for _, img := range images {
		actions[img.ID()] = shared.ImageActionPending
	}
	return &Session{
		id:           id,
		directory:    directory,
		filter:       filter,
		targetKeep:   targetKeep,
		createdAt:    time.Now(),
		updatedAt:    time.Now(),
		queue:        images,
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

func (s *Session) Directory() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.directory
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

func (s *Session) Status() shared.SessionStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats().Status()
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

func (s *Session) CurrentIndex() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentIdx
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
		// 保存当前索引时，如果已经处理完所有图片，则保存最后一张图片的索引
		saveIdx := s.currentIdx
		if saveIdx >= len(s.queue) && len(s.queue) > 0 {
			saveIdx = len(s.queue) - 1
		}
		s.roundHistory = append(s.roundHistory, RoundSnapshot{
			queue:      s.queue,
			currentIdx: saveIdx,
			undoStack:  s.undoStack,
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
	stats.processed = s.currentIdx
	stats.remaining = len(s.queue) - s.currentIdx
	stats.targetKeep = s.targetKeep

	for i := 0; i < s.currentIdx; i++ {
		img := s.queue[i]
		action := s.actions[img.ID()]
		switch action {
		case shared.ImageActionKeep:
			stats.kept++
		case shared.ImageActionPending:
			stats.reviewed++
		case shared.ImageActionReject:
			stats.rejected++
		}
	}

	stats.rejected += len(s.actions) - len(s.queue)

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

	stats := s.Stats()
	if stats.processed > 0 {
		return true
	}

	return len(s.actions) > len(s.queue)
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
			return ErrSessionNotFound
		}
	}

	s.undoStack = append(s.undoStack, UndoEntry{
		imageID: imageID,
		action:  s.actions[imageID],
	})

	s.actions[imageID] = action
	s.updatedAt = time.Now()

	s.currentIdx++

	if s.currentIdx >= len(s.queue) {
		stats := s.stats()

		if stats.reviewed > 0 || stats.kept > 0 {
			var newQueue []*image.Image
			for _, img := range s.queue {
				action := s.actions[img.ID()]
				if action == shared.ImageActionPending || action == shared.ImageActionKeep {
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
		s.currentIdx = lastRound.currentIdx
		s.undoStack = lastRound.undoStack
		s.updatedAt = time.Now()
		return nil
	}

	// 普通撤销
	lastEntry := s.undoStack[len(s.undoStack)-1]
	s.undoStack = s.undoStack[:len(s.undoStack)-1]

	s.actions[lastEntry.imageID] = lastEntry.action

	s.currentIdx--
	s.updatedAt = time.Now()
	return nil
}

func (s *Session) Images() []*image.Image {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.roundHistory) == 0 {
		// 当前是第一轮，队列就是所有图片
		return s.queue
	}
	// 返回第一轮的图片队列
	return s.roundHistory[0].queue
}

func (s *Session) GetAction(imageID scalar.ID) shared.ImageAction {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if action, exists := s.actions[imageID]; exists {
		return action
	}
	return shared.ImageActionPending
}

func (s *Session) SetAction(imageID scalar.ID, action shared.ImageAction) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.actions[imageID] = action
}

// UndoEntry 表示一个可撤销的操作条目，用于存储图片操作的历史状态
// 当执行撤销操作时，使用此条目恢复图片的原始操作状态

type UndoEntry struct {
	imageID scalar.ID
	action  shared.ImageAction
}

var (
	ErrSessionNotActive = &SessionError{message: "session is not active"}
	ErrNoMoreImages     = &SessionError{message: "no more images"}
	ErrSessionNotFound  = &SessionError{message: "session not found"}
	ErrNothingToUndo    = &SessionError{message: "nothing to undo"}
)

type SessionError struct {
	message string
}

func (e *SessionError) Error() string {
	return e.message
}
