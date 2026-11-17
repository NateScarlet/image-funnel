package session

import (
	"main/internal/domain/image"
	"main/internal/shared"
	"main/internal/util"
)

// remainingInRound 返回本轮队列中尚未处理的图片数量，
// 是「本轮是否走完」算术的唯一来源，供 Stats 与自动推进共用。
func (s *Session) remainingInRound() int {
	return len(s.queue) - s.currentIdx
}

// currentRoundDone 判断本轮是否已处理完
func (s *Session) currentRoundDone() bool {
	return s.remainingInRound() <= 0
}

// Stats 计算会话的统计信息，包括处理进度和各种操作的图片数量
func (s *Session) Stats() *shared.StatsDTO {
	var stats shared.StatsDTO
	stats.TotalCount = len(s.queue)
	stats.CurrentRoundRemaining = s.remainingInRound()

	filterFunc := s.imageFilterBuilder.Build(util.UnwrapPointer(s.filter))

	for id, action := range s.actions {
		// 如果该图片已被强制删除，跳过
		if s.removed != nil && s.removed[id] {
			continue
		}

		// 查找对应的图片对象
		var img *image.Image
		if idx, ok := s.indexByID[id]; ok {
			img = s.images[idx]
		}

		// 如果找不到图片（不应该发生，除非数据不一致），跳过
		if img == nil {
			continue
		}

		// 确保只计算符合当前过滤条件的图片，或者本会话已提交过的图片
		// 这样既能过滤掉因 Update 修改 filter 而被排除的图片，
		// 又能保留 Commit 后 Rating 改变但仍保留在会话中的已操作图片。
		if filterFunc(img) || s.committed[id] {
			switch action {
			case shared.ImageActionKeep:
				stats.TotalKept++
			case shared.ImageActionShelve:
				stats.TotalShelved++
			case shared.ImageActionReject:
				stats.TotalRejected++
			}
		}
	}

	// 计算isCompleted字段
	// 会话完成条件：
	// 1. 所有图片都已处理 (remaining == 0)
	// 2. 且保留的图片数量不超过目标保留数量 (否则需要开启新一轮)
	// 注意：搁置 (Shelve) 的图片不计入目标保留数量计算，因为它们在本会话中被视为已丢弃
	stats.IsCompleted = s.currentRoundDone() && (stats.TotalKept <= s.targetKeep)

	return &stats
}
