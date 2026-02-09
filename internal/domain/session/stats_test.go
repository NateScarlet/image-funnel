package session

import (
	"main/internal/domain/image"
	"main/internal/scalar"
	"main/internal/shared"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStats_InitialState(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	stats := session.Stats()

	assert.Equal(t, 10, stats.Total, "Total should be 10")
	assert.Equal(t, 0, session.CurrentIndex(), "Processed should be 0")
	assert.Equal(t, 0, stats.Kept, "Kept should be 0")
	assert.Equal(t, 0, stats.Shelved, "Shelved should be 0")
	assert.Equal(t, 0, stats.Rejected, "Rejected should be 0")
	assert.Equal(t, 10, stats.Remaining, "Remaining should be 10")
}

func TestStats_AfterMarkingImages(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	// 标记前3张为 KEEP
	for i := 0; i < 3; i++ {
		err := session.MarkImage(session.images[session.queue[i]].ID(), shared.ImageActionKeep)
		require.NoError(t, err)
	}

	// 标记中间3张为 SHELVE
	for i := 3; i < 6; i++ {
		err := session.MarkImage(session.images[session.queue[i]].ID(), shared.ImageActionShelve)
		require.NoError(t, err)
	}

	// 标记后3张为 REJECT
	for i := 6; i < 9; i++ {
		err := session.MarkImage(session.images[session.queue[i]].ID(), shared.ImageActionReject)
		require.NoError(t, err)
	}

	stats := session.Stats()

	assert.Equal(t, 10, stats.Total, "Total should be 10")
	assert.Equal(t, 9, session.CurrentIndex(), "Processed should be 9")
	assert.Equal(t, 3, stats.Kept, "Kept should be 3")
	assert.Equal(t, 3, stats.Shelved, "Shelved should be 3")
	assert.Equal(t, 3, stats.Rejected, "Rejected should be 3")
	assert.Equal(t, 1, stats.Remaining, "Remaining should be 1")
}

func TestStats_CurrentFilterExcludesKeptImage(t *testing.T) {
	// 5 images with rating 1, 2, 3, 4, 5
	images := createTestImagesWithRatings([]int{1, 2, 3, 4, 5})
	// Target keep 1
	session := NewSession(scalar.ToID("s1"), scalar.ToID("d1"), nil, 1, images)

	// Keep image with rating 3 (index 2)
	require.NoError(t, session.MarkImage(images[2].ID(), shared.ImageActionKeep))

	// Stats: Kept=1
	assert.Equal(t, 1, session.Stats().Kept)

	// Create filter for Rating=4,5
	filter := &shared.ImageFilters{Rating: []int{4, 5}}

	// NextRound with filtered images
	filteredImages := []*image.Image{images[3], images[4]}
	require.NoError(t, session.NextRound(filter, filteredImages))

	// Expected: Kept should be 0 because the kept image (rating 3) is excluded by filter (4,5)
	assert.Equal(t, 0, session.Stats().Kept)
}

func TestMarkImage_AllImagesRejected_ShouldCompleteSession(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	markImagesInSession(t, session, func(index int) shared.ImageAction {
		return shared.ImageActionReject
	})

	assert.True(t, session.Stats().IsCompleted, "IsCompleted should be true")
}

func TestMarkImage_KeptLessOrEqualTarget_ShouldComplete(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	markImagesInSession(t, session, func(index int) shared.ImageAction {
		action := shared.ImageActionKeep
		if index >= 5 {
			action = shared.ImageActionReject
		}
		return action
	})

	assert.True(t, session.Stats().IsCompleted, "Session should be completed when kept <= target")
}

func TestSession_ShouldCompleteWhenKeepTargetReached(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	markImagesInSession(t, session, func(index int) shared.ImageAction {
		action := shared.ImageActionKeep
		if index >= 5 {
			action = shared.ImageActionReject
		}
		return action
	})

	assert.True(t, session.Stats().IsCompleted, "Session should be completed when kept == target")
}

func TestMarkImage_WithShelvedAndKept_ShouldCompleteIfKeptBelowTarget(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	markImagesInSession(t, session, func(index int) shared.ImageAction {
		if index < 2 {
			return shared.ImageActionKeep
		}
		return shared.ImageActionShelve
	})

	stats := session.Stats()
	assert.Equal(t, 0, stats.Remaining, "All images should be processed")
	assert.Equal(t, 2, stats.Kept, "Should have 2 kept images")
	assert.Equal(t, 8, stats.Shelved, "Should have 8 shelved images")
	assert.True(t, stats.IsCompleted, "Session should be completed because shelved images are ignored")
}
