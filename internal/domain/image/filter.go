package image

import (
	"main/internal/shared"
)

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

func FilterImages(images []*Image, filterFunc func(*Image) bool) []*Image {
	if filterFunc == nil {
		return images
	}

	result := make([]*Image, 0, len(images))
	for _, img := range images {
		if filterFunc(img) {
			result = append(result, img)
		}
	}
	return result
}
