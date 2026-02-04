package session

import (
	"main/internal/domain/image"
	"main/internal/scalar"
	"time"
)

// #region 维护方法

// UpdateImageByPath 根据路径更新图片信息
func (s *Session) UpdateImageByPath(img *image.Image, matchesFilter bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	var oldID scalar.ID
	var oldIndex = -1
	for id, existing := range s.images {
		if existing.Path() == img.Path() {
			oldID = id
			break
		}
	}

	for i, existing := range s.queue {
		if existing.Path() == img.Path() {
			oldIndex = i
			break
		}
	}

	// 如果原本不在会话中
	if oldID.IsZero() && oldIndex == -1 {
		if matchesFilter {
			s.addFilteredImageLocked(img)
			return true
		}
		return false
	}

	// 如果不匹配过滤器，从会话中移除
	if !matchesFilter {
		return s.removeImageByPathLocked(img.Path())
	}

	// 更新图片信息
	if !oldID.IsZero() {
		delete(s.images, oldID)
		s.images[img.ID()] = img

		// 如果 ID 发生了变化（通常是由于修改时间变化）
		if oldID != img.ID() {
			if action, ok := s.actions[oldID]; ok {
				s.actions[img.ID()] = action
				delete(s.actions, oldID)
			}
		}
	}

	if oldIndex != -1 {
		s.queue[oldIndex] = img
	}

	s.updatedAt = time.Now()
	return true
}

// RemoveImageByPath 根据路径从会话中移除图片
func (s *Session) RemoveImageByPath(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.removeImageByPathLocked(path)
}

func (s *Session) removeImageByPathLocked(path string) bool {
	var targetID scalar.ID
	var targetIndex = -1
	for id, img := range s.images {
		if img.Path() == path {
			targetID = id
			break
		}
	}

	if targetID.IsZero() {
		return false
	}

	for i, img := range s.queue {
		if img.ID() == targetID {
			targetIndex = i
			break
		}
	}

	delete(s.images, targetID)
	delete(s.actions, targetID)

	if targetIndex != -1 {
		s.queue = append(s.queue[:targetIndex], s.queue[targetIndex+1:]...)
		if targetIndex < s.currentIdx {
			s.currentIdx--
		}

	}

	s.updatedAt = time.Now()
	return true
}

func (s *Session) addFilteredImageLocked(img *image.Image) error {
	// 检查是否已经在队列中
	if _, existing := s.images[img.ID()]; existing {
		return nil
	}

	s.queue = append(s.queue, img)
	s.images[img.ID()] = img
	s.updatedAt = time.Now()

	return nil
}

// #endregion
