package image

import (
	"main/internal/shared"
)

// TODO: refactor to filter builder
func BuildImageFilter(filter *shared.ImageFilters) func(*Image) bool {
	if filter == nil || len(filter.Rating) == 0 {
		return func(img *Image) bool {
			return img != nil
		}
	}

	allowedRatings := make(map[int]bool)
	for _, r := range filter.Rating {
		allowedRatings[r] = true
	}

	return func(img *Image) bool {
		if img == nil {
			return false
		}
		return allowedRatings[img.Rating()]
	}
}
