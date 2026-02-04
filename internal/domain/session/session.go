package session

import (
	"iter"
	"main/internal/apperror"
	"main/internal/domain/image"
	"main/internal/scalar"
	"main/internal/shared"
	"sync"
	"time"
)

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
	undoStack  []func()                         // 撤销操作栈
	actions    map[scalar.ID]shared.ImageAction // 图片操作映射
	durations  map[scalar.ID]scalar.Duration    // 图片操作耗时映射

	currentRound int // 当前筛选轮次

	mu sync.RWMutex // 读写互斥锁，用于并发安全访问
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
		undoStack:    make([]func(), 0),
		actions:      actions,
		durations:    make(map[scalar.ID]scalar.Duration),
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

var (
	ErrNoMoreImages  = apperror.New("INVALID_OPERATION", "no more images", "没有更多图片")
	ErrNothingToUndo = apperror.New("INVALID_OPERATION", "nothing to undo", "没有可以撤销的操作")
)
