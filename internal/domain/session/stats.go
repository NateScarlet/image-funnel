package session

import (
	"main/internal/domain/image"
	"main/internal/shared"
)

// Stats 计算会话的统计信息，包括处理进度和各种操作的图片数量
func (s *Session) Stats() *shared.StatsDTO {
	var stats shared.StatsDTO
	stats.Total = len(s.queue)
	stats.Remaining = len(s.queue) - s.currentIdx

	filterFunc := image.BuildImageFilter(s.filter)

	for id, action := range s.actions {
		// 查找对应的图片对象
		var img *image.Image
		if idx, ok := s.indexByID[id]; ok {
			img = s.images[idx]
		}

		// 如果找不到图片（不应该发生，除非数据不一致），跳过
		if img == nil {
			continue
		}

		// 确保只计算符合当前过滤条件的图片
		// 如果图片因为过滤条件改变而不再可见，它不应该计入当前的保留/搁置等统计
		// 这样可以避免用户因“看不见但已保留”的图片而无法完成会话
		if filterFunc(img) {
			switch action {
			case shared.ImageActionKeep:
				stats.Kept++
			case shared.ImageActionShelve:
				stats.Shelved++
			case shared.ImageActionReject:
				stats.Rejected++
			}
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
