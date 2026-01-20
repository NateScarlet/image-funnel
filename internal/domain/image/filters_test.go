package image

import (
	"fmt"
	"main/internal/domain/metadata"
	"main/internal/scalar"
	"main/internal/shared"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func filterImages(images []*Image, filterFunc func(*Image) bool) []*Image {
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

func TestImageFilters_Rating(t *testing.T) {
	filter := &shared.ImageFilters{Rating: []int{0, 1, 2}}

	assert.Equal(t, []int{0, 1, 2}, filter.Rating, "Rating should match")
}

func TestBuildImageFilter_WithRating(t *testing.T) {
	images := createTestImagesWithRatings([]int{0, 1, 2, 3, 4, 0, 1, 2, 3, 4})

	filter := &shared.ImageFilters{Rating: []int{0, 1}}
	filterFunc := BuildImageFilter(filter)
	filtered := filterImages(images, filterFunc)

	assert.Equal(t, 4, len(filtered), "Should filter to 4 images with rating 0 or 1")
	for _, img := range filtered {
		assert.Contains(t, []int{0, 1}, img.Rating(), "Image rating should be 0 or 1")
	}
}

func TestBuildImageFilter_WithNilFilter(t *testing.T) {
	images := createTestImagesWithRatings([]int{0, 1, 2, 3, 4, 5})

	filterFunc := BuildImageFilter(nil)
	filtered := filterImages(images, filterFunc)

	assert.Equal(t, len(images), len(filtered), "Should not filter with nil filter")
}

func TestBuildImageFilter_WithEmptyRating(t *testing.T) {
	images := createTestImagesWithRatings([]int{0, 1, 2, 3, 4, 5})

	filter := &shared.ImageFilters{Rating: []int{}}
	filterFunc := BuildImageFilter(filter)
	filtered := filterImages(images, filterFunc)

	assert.Equal(t, 6, len(filtered), "Should include all images when rating is empty")
}

func TestBuildImageFilter_WithSingleRating(t *testing.T) {
	images := createTestImagesWithRatings([]int{0, 1, 2, 3, 4, 5, 2, 2})

	filter := &shared.ImageFilters{Rating: []int{2}}
	filterFunc := BuildImageFilter(filter)
	filtered := filterImages(images, filterFunc)

	assert.Equal(t, 3, len(filtered), "Should filter to 3 images with rating 2")
	for _, img := range filtered {
		assert.Equal(t, 2, img.Rating(), "All images should have rating 2")
	}
}

func TestFilterImages_WithNilFilter(t *testing.T) {
	images := createTestImagesWithRatings([]int{0, 1, 2, 3, 4, 5})

	filtered := filterImages(images, nil)

	assert.Equal(t, 6, len(filtered), "Should return all images when filter is nil")
}

func createTestImagesWithRatings(ratings []int) []*Image {
	images := make([]*Image, len(ratings))
	for i, rating := range ratings {
		xmpData := metadata.NewXMPData(rating, "", time.Time{})
		images[i] = NewImage(
			scalar.ToID(fmt.Sprintf("img-%d", i)),
			"test.jpg",
			fmt.Sprintf("/test/test-%d.jpg", i),
			1000,
			time.Now(),
			xmpData,
			1920,
			1080,
		)
	}
	return images
}
