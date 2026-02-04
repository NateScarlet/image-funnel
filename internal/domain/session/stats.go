package session

import (
	"main/internal/shared"
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

// Stats 表示会话的统计信息，用于跟踪筛选进度和结果
type Stats struct {
	total       int
	kept        int
	shelved     int
	rejected    int
	remaining   int
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

func (s *Stats) IsCompleted() bool {
	return s.isCompleted
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

	for _, action := range s.actions {
		switch action {
		case shared.ImageActionKeep:
			stats.kept++
		case shared.ImageActionShelve:
			stats.shelved++
		case shared.ImageActionReject:
			stats.rejected++
		}
	}

	// 计算isCompleted字段
	// 会话完成条件：
	// 1. 所有图片都已处理 (remaining == 0)
	// 2. 且保留的图片数量不超过目标保留数量 (否则需要开启新一轮)
	// 注意：搁置 (Shelve) 的图片不计入目标保留数量计算，因为它们在本会话中被视为已丢弃
	stats.isCompleted = stats.remaining == 0 && (stats.kept <= s.targetKeep)

	return &stats
}
