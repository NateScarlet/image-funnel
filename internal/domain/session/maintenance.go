package session

import (
	"main/internal/domain/image"
	"time"
)

// #region 维护方法

// UpdateImageByPath 根据路径更新图片信息
func (s *Session) UpdateImageByPath(img *image.Image, matchesFilter bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 从 map 中获取索引
	idx, ok := s.indexByPath[img.Path()]
	var oldIndex = -1      // 在 queue 中的索引
	var oldImageIndex = -1 // 在 images 中的索引

	// 在 queue 中查找匹配该路径的图片
	for i, imgIndex := range s.queue {
		if s.images[imgIndex].Path() == img.Path() {
			oldIndex = i
			oldImageIndex = imgIndex
			break
		}
	}

	// 如果 queue 里没找到，尝试通过 Path 找
	if oldImageIndex == -1 && ok {
		oldImageIndex = idx
	}

	// 如果原本不在会话中（既不在 queue 也不在 history）
	if oldImageIndex == -1 {
		if matchesFilter {
			s.addFilteredImageLocked(img)
			return true
		}
		return false
	}

	// 如果不匹配过滤器，从 queue 中移除
	if !matchesFilter {
		return s.removeImageByPathLocked(img.Path())
	}

	// 匹配过滤器，执行更新
	// 更新 images 中的对象
	// 如果 ID 没变，直接更新对象内容
	oldImg := s.images[oldImageIndex]
	if oldImg.ID() == img.ID() {
		s.images[oldImageIndex] = img
		// queue 中的引用是 index，不需要变
	} else {
		// ID 变了 (如修改时间变化)，视为新图片
		// 添加新图片到 images
		newImageIndex := len(s.images)
		s.images = append(s.images, img)
		s.indexByID[img.ID()] = newImageIndex
		s.indexByPath[img.Path()] = newImageIndex

		// 迁移 action
		if action, ok := s.actions[oldImg.ID()]; ok {
			s.actions[img.ID()] = action
			delete(s.actions, oldImg.ID())
		}

		// 更新 queue 指向新图片
		if oldIndex != -1 {
			s.queue[oldIndex] = newImageIndex
		}
	}

	s.updatedAt = time.Now()
	return true
}

// BatchUpdateImages 批量更新图片信息
func (s *Session) BatchUpdateImages(images []*image.Image) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, img := range images {
		// 1. 尝试通过 ID 查找
		// 这是最快的方式，也是主要路径
		if idx, ok := s.indexByID[img.ID()]; ok {
			// 如果它是已存在的旧图片，我们只更新内容
			s.images[idx] = img

			// 对于 Commit 产生的更新，通常 path 不变 (或我们只得到 id)
			// 如果 Path 变了，我们需要更新 indexByPath ... 但这里假定 invariant
			continue
		}

		// 2. 如果是新的图片 (例如如果是首次导入?) 但 Commit 不会产生新图片 unless ID changed
		// 如果 ID 变了，我们需要添加新图片
		newIdx := len(s.images)
		s.images = append(s.images, img)
		s.indexByID[img.ID()] = newIdx
		s.indexByPath[img.Path()] = newIdx

		// 注意：这里的 context 是 "Commit 后更新内存状态"
		// 原始代码是 UpdateImageByPath(img, true) -> 也就意味着如果是新图片，加入 queue
		// 我在这里简化逻辑：如果是 Commit 产生的 "新" 图片 (ID 不匹配任何现有)，
		// 我们将其加入 queue (因为 matchesFilter=true)
		s.queue = append(s.queue, newIdx)
	}
	s.updatedAt = time.Now()
}

// RemoveImageByPath 根据路径从会话中移除图片
func (s *Session) RemoveImageByPath(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.removeImageByPathLocked(path)
}

func (s *Session) removeImageByPathLocked(path string) bool {
	var targetIndex = -1

	// 查找 queue 中的索引
	for i, imgIndex := range s.queue {
		if s.images[imgIndex].Path() == path {
			targetIndex = i
			break
		}
	}

	if targetIndex == -1 {
		return false
	}

	// 从 queue 中移除
	s.queue = append(s.queue[:targetIndex], s.queue[targetIndex+1:]...)
	if targetIndex < s.currentIdx {
		s.currentIdx--
	}

	// 注意：我们不从 s.images 中移除图片，保持“只增不减”并维持索引稳定性

	s.updatedAt = time.Now()
	return true
}

func (s *Session) addFilteredImageLocked(img *image.Image) error {
	// 检查 ID 是否已存在
	if idx, ok := s.indexByID[img.ID()]; ok {
		// 只是更新引用，不添加到队列
		// 如果它已经存在但不在队列中，说明它已经被处理过（保留/排除/搁置）
		// 我们保留这个决定，不重新将其加入队列
		s.images[idx] = img
		return nil
	}

	// 新增
	newIdx := len(s.images)
	s.images = append(s.images, img)
	s.indexByID[img.ID()] = newIdx
	s.indexByPath[img.Path()] = newIdx
	s.queue = append(s.queue, newIdx)

	s.updatedAt = time.Now()

	return nil
}

// #endregion
