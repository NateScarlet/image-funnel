package session

import (
	"iter"
	"main/internal/apperror"
	"main/internal/domain/image"
	"main/internal/scalar"
	"main/internal/shared"
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

	images      []*image.Image    // 会话所有图片集合（只增不减，引用稳定）
	indexByID   map[scalar.ID]int // ID -> images索引映射
	indexByPath map[string]int    // Path -> images索引映射（最新版本）
	queue       []int             // 待处理队列（存储 images 索引）

	currentIdx int                              // 当前处理的图片在队列中的索引
	undoStack  []func()                         // 撤销操作栈
	actions    map[scalar.ID]shared.ImageAction // 图片操作映射
	durations  map[scalar.ID]scalar.Duration    // 图片操作耗时映射

	currentRound int // 当前筛选轮次
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
	indexByID := make(map[scalar.ID]int, len(images))
	indexByPath := make(map[string]int, len(images))
	queue := make([]int, len(images))

	for i, img := range images {
		indexByID[img.ID()] = i
		indexByPath[img.Path()] = i
		queue[i] = i
	}

	return &Session{
		id:           id,
		directoryID:  directoryID,
		filter:       filter,
		targetKeep:   targetKeep,
		createdAt:    time.Now(),
		updatedAt:    time.Now(),
		images:       images,
		indexByID:    indexByID,
		indexByPath:  indexByPath,
		queue:        queue,
		currentIdx:   0,
		undoStack:    make([]func(), 0),
		actions:      actions,
		durations:    make(map[scalar.ID]scalar.Duration),
		currentRound: 0,
	}
}

func (s *Session) ID() scalar.ID {
	return s.id
}

func (s *Session) DirectoryID() scalar.ID {
	return s.directoryID
}

func (s *Session) Filter() *shared.ImageFilters {
	return s.filter
}

func (s *Session) TargetKeep() int {
	return s.targetKeep
}

func (s *Session) CreatedAt() time.Time {
	return s.createdAt
}

func (s *Session) UpdatedAt() time.Time {
	return s.updatedAt
}

func (s *Session) Actions() iter.Seq2[*image.Image, shared.ImageAction] {
	return func(yield func(*image.Image, shared.ImageAction) bool) {
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
