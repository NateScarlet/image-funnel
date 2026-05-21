package image

import (
	"main/internal/scalar"
	"main/internal/shared"
	"strings"
)

// TODO: refactor to filter builder
func BuildImageFilter(filter *shared.ImageFilters) func(*Image) bool {
	if filter == nil {
		return func(img *Image) bool {
			return img != nil
		}
	}

	// 准备ID过滤条件
	hasID := len(filter.ID) > 0
	allowedIDs := make(map[scalar.ID]bool)
	for _, id := range filter.ID {
		allowedIDs[id] = true
	}

	// 准备目录ID过滤条件
	hasDirectoryID := len(filter.DirectoryID) > 0
	allowedDirectoryIDs := make(map[scalar.ID]bool)
	for _, id := range filter.DirectoryID {
		allowedDirectoryIDs[id] = true
	}

	// 准备星级过滤条件
	hasRating := len(filter.Rating) > 0
	allowedRatings := make(map[int]bool)
	for _, r := range filter.Rating {
		allowedRatings[r] = true
	}

	// 准备颜色标签过滤条件（忽略大小写）
	hasLabel := len(filter.Label) > 0
	allowedLabels := make(map[string]bool)
	for _, l := range filter.Label {
		allowedLabels[strings.ToLower(l)] = true
	}

	// 准备文本搜索条件（忽略大小写，匹配文件名）
	hasQuery := filter.Query != ""
	queryLower := strings.ToLower(filter.Query)

	return func(img *Image) bool {
		if img == nil {
			return false
		}
		if hasID && !allowedIDs[img.ID()] {
			return false
		}
		if hasDirectoryID && !allowedDirectoryIDs[img.DirectoryID()] {
			return false
		}
		if hasRating && !allowedRatings[img.Rating()] {
			return false
		}
		if hasLabel && !allowedLabels[strings.ToLower(img.Label())] {
			return false
		}
		if hasQuery && !strings.Contains(strings.ToLower(img.Filename()), queryLower) {
			return false
		}
		return true
	}
}
