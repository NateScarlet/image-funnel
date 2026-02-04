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

func (s *Session) Stats() *shared.StatsDTO {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats()
}

// stats 计算会话的统计信息，包括处理进度和各种操作的图片数量
//
// 注意：此方法是内部方法，不进行并发控制，调用者需要自行保证线程安全
func (s *Session) stats() *shared.StatsDTO {
	var stats shared.StatsDTO
	stats.Total = len(s.queue)
	stats.Remaining = len(s.queue) - s.currentIdx

	for _, action := range s.actions {
		switch action {
		case shared.ImageActionKeep:
			stats.Kept++
		case shared.ImageActionShelve:
			stats.Shelved++
		case shared.ImageActionReject:
			stats.Rejected++
		}
	}

	// 计算isCompleted字段
	// 会话完成条件：
	// 1. 所有图片都已处理 (remaining == 0)
	// 2. 且保留的图片数量不超过目标保留数量 (否则需要开启新一轮)
	// 注意：搁置 (Shelve) 的图片不计入目标保留数量计算，因为它们在本会话中被视为已丢弃
	stats.IsCompleted = stats.Remaining == 0 && (stats.Kept <= s.targetKeep)

	return &stats
}
