package session

import (
	"main/internal/apperror"
	"main/internal/domain/image"
	"main/internal/domain/relpath"
	"main/internal/scalar"
	"main/internal/shared"
	"slices"
	"sort"
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

	images         []*image.Image       // 会话所有图片集合（只增不减，引用稳定）
	indexByID      map[scalar.ID]int    // ID -> images索引映射
	indexByRelPath map[relpath.Path]int // RelPath -> images索引映射（最新版本）
	queue          []int                // 待处理队列（存储 images 索引）

	currentIdx int                              // 当前处理的图片在队列中的索引
	undoStack  []func()                         // 撤销操作栈
	actions    map[scalar.ID]shared.ImageAction // 图片操作映射
	durations  map[scalar.ID]scalar.Duration    // 图片操作耗时映射
	samples    []*Sample                        // 会话中存活的样本载荷（Mark 产生、Undo 移除、Commit 提交后清空）

	currentRound       int                // 当前筛选轮次
	committed          map[scalar.ID]bool // 记录已提交过的图片 ID
	imageFilterBuilder *image.FilterBuilder
	removed            map[scalar.ID]bool // 记录被物理移除的图片 ID，防止其 action 影响统计数据
}

// New 创建一个新的图片筛选会话
//
// 参数：
// - id: 会话唯一标识符
// - directoryID: 目录 ID
// - filter: 图片过滤器
// - targetKeep: 目标保留图片数量
// - images: 待处理的图片集合
func New(id scalar.ID, directoryID scalar.ID, filter *shared.ImageFilters, targetKeep int, images []*image.Image, imageFilterBuilder *image.FilterBuilder) *Session {
	actions := make(map[scalar.ID]shared.ImageAction)
	indexByID := make(map[scalar.ID]int, len(images))
	indexByRelPath := make(map[relpath.Path]int, len(images))
	queue := make([]int, len(images))

	for i, img := range images {
		indexByID[img.ID()] = i
		indexByRelPath[img.RelPath()] = i
		queue[i] = i
	}

	return &Session{
		id:                 id,
		directoryID:        directoryID,
		filter:             filter,
		targetKeep:         targetKeep,
		createdAt:          time.Now(),
		updatedAt:          time.Now(),
		images:             images,
		indexByID:          indexByID,
		indexByRelPath:     indexByRelPath,
		queue:              queue,
		currentIdx:         0,
		undoStack:          make([]func(), 0),
		actions:            actions,
		durations:          make(map[scalar.ID]scalar.Duration),
		samples:            make([]*Sample, 0),
		currentRound:       0,
		committed:          make(map[scalar.ID]bool),
		imageFilterBuilder: imageFilterBuilder,
		removed:            make(map[scalar.ID]bool),
	}
}

// MarkCommitted 记录图片在本会话中已提交过
func (s *Session) MarkCommitted(id scalar.ID) {
	if s.committed == nil {
		s.committed = make(map[scalar.ID]bool)
	}
	s.committed[id] = true
}

// IsCommitted 判断图片在本会话中是否已经提交过
func (s *Session) IsCommitted(id scalar.ID) bool {
	if s.committed == nil {
		return false
	}
	return s.committed[id]
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

// CurrentImage 返回当前正在处理的图片
func (s *Session) CurrentImage() *image.Image {
	if s.currentIdx < len(s.queue) {
		return s.images[s.queue[s.currentIdx]]
	}
	return nil
}

// NextImages 返回指定数量的后续图片
func (s *Session) NextImages(count int) []*image.Image {
	if count == 0 {
		return nil
	}
	if count < 0 {
		// 返回所有
		indices := s.queue[s.currentIdx+1:]
		imgs := make([]*image.Image, len(indices))
		for i, idx := range indices {
			imgs[i] = s.images[idx]
		}
		return imgs
	}
	start := s.currentIdx + 1
	if start >= len(s.queue) {
		return nil
	}
	end := start + count
	if end > len(s.queue) {
		end = len(s.queue)
	}

	indices := s.queue[start:end]
	imgs := make([]*image.Image, len(indices))
	for i, idx := range indices {
		imgs[i] = s.images[idx]
	}
	return imgs
}

// knownImages 返回会话已知的全部图片（只增不减，含已处理与已被移出队列的）。
// 改筛选以这些图片为候选集，磁盘读盘仅用于刷新它们的元数据引用。
func (s *Session) knownImages() []*image.Image {
	return slices.Clone(s.images)
}

// KeptImages 返回所有已被标记为保留的图片
func (s *Session) KeptImages(limit, offset int) []*image.Image {

	var kept []*image.Image
	for _, img := range s.images {
		if s.removed != nil && s.removed[img.ID()] {
			continue
		}
		if s.actions[img.ID()] == shared.ImageActionKeep {
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

// #region 样本载荷

// baselinesForMark 读取本次 Mark 之前会话内全部 KEEP 图作为基准集合，并排除当前图自己。
// 基准在 Mark 当下从会话状态读取，不从回收站推断；当前图若曾被判 KEEP，这次被重新标记时
// 必须排除，否则会出现「拿自己和自己比」的样本。
func (s *Session) baselinesForMark(currentID scalar.ID) []SampleImage {
	kept := s.KeptImages(-1, 0)
	baselines := make([]SampleImage, 0, len(kept))
	for _, img := range kept {
		if img.ID() == currentID {
			continue
		}
		baselines = append(baselines, SampleImage{ID: img.ID(), RelPath: img.RelPath()})
	}
	return baselines
}

// LiveSamples 返回会话当前存活的样本载荷副本。
func (s *Session) LiveSamples() []*Sample {
	return slices.Clone(s.samples)
}

// ClearSamples 在 Commit 把载荷交给流水线后清空，避免重复 Commit 产生重复归档。
func (s *Session) ClearSamples() {
	s.samples = nil
}

// removeSample 按指针身份移除一条载荷；载荷已被 ClearSamples 清空时为空操作。
func (s *Session) removeSample(sample *Sample) {
	s.samples = slices.DeleteFunc(s.samples, func(existing *Sample) bool { return existing == sample })
}

// #endregion

// CurrentIndex 返回当前处理图片的索引
func (s *Session) CurrentIndex() int {
	return s.currentIdx
}

// CurrentSize 返回当前队列的总图片数量
func (s *Session) CurrentSize() int {
	return len(s.queue)
}

// CurrentRound 返回当前筛选轮次
func (s *Session) CurrentRound() int {
	return s.currentRound
}

// CurrentRoundActions 返回当前队列中所有图片的操作状态
func (s *Session) CurrentRoundActions() []shared.ImageAction {
	actions := make([]shared.ImageAction, s.currentIdx)
	for i := 0; i < s.currentIdx; i++ {
		idx := s.queue[i]
		actions[i] = s.actions[s.images[idx].ID()]
	}
	return actions
}

var (
	ErrNoMoreImages  = apperror.New("INVALID_OPERATION", "no more images", "没有更多图片")
	ErrNothingToUndo = apperror.New("INVALID_OPERATION", "nothing to undo", "没有可以撤销的操作")
)
