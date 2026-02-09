package session

import (
	"main/internal/domain/image"
	"time"
)

// UpdateImage 根据路径更新图片信息
func (s *Session) UpdateImage(img *image.Image, matchesFilter bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.unsafeUpdateImage(img, matchesFilter)
}

// unsafeUpdateImage 无锁版本的图片更新逻辑
// 调用者必须持有写锁
func (s *Session) unsafeUpdateImage(img *image.Image, matchesFilter bool) bool {
	// 从 map 中获取索引
	idx, ok := s.indexByPath[img.Path()]
	var oldQueueIndex = -1 // 在 queue 中的索引
	var oldImageIndex = -1 // 在 images 中的索引

	// 在 queue 中查找匹配该路径的图片
	for i, imgIndex := range s.queue {
		if s.images[imgIndex].Path() == img.Path() {
			oldQueueIndex = i
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
			s.unsafeAddFilteredImage(img)
			return true
		}
		return false
	}

	// 如果不匹配过滤器，从 queue 中移除
	if !matchesFilter {
		return s.unsafeRemoveImageByPath(img.Path())
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
		if oldQueueIndex != -1 {
			s.queue[oldQueueIndex] = newImageIndex
		}
	}

	s.updatedAt = time.Now()
	return true
}

// RemoveImageByPath 根据路径从会话中移除图片
func (s *Session) RemoveImageByPath(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.unsafeRemoveImageByPath(path)
}

func (s *Session) unsafeRemoveImageByPath(path string) bool {
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

func (s *Session) unsafeAddFilteredImage(img *image.Image) error {
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
