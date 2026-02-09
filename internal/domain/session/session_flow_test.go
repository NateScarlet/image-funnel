package session

import (
	"main/internal/domain/image"
	"main/internal/scalar"
	"main/internal/shared"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarkImage_AllImagesRejected_ShouldCompleteSession(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	markImagesInSession(t, session, func(index int) shared.ImageAction {
		return shared.ImageActionReject
	})

	assert.True(t, session.Stats().IsCompleted, "IsCompleted should be true")
}

func TestMarkImage_KeepAndReview_ShouldStartNextRound(t *testing.T) {
	session := setupTestSession(t, 10, 2)

	markImagesInSession(t, session, func(index int) shared.ImageAction {
		action := shared.ImageActionKeep
		switch index % 3 {
		case 0:
			action = shared.ImageActionShelve
		case 1:
			action = shared.ImageActionReject
		}
		return action
	})

	assert.False(t, session.Stats().IsCompleted, "Session should not be completed")

	newRoundStats := session.Stats()
	assert.Equal(t, 0, session.CurrentIndex(), "New round processed should be 0")
	assert.Equal(t, 3, newRoundStats.Total, "New round total should be 3")
	assert.Equal(t, 0, session.CurrentIndex(), "CurrentIdx should be 0")
}

func TestMarkImage_KeepAndReview_ShouldStartNextRoundWithBoth(t *testing.T) {
	session := setupTestSession(t, 10, 2)

	markImagesInSession(t, session, func(index int) shared.ImageAction {
		action := shared.ImageActionKeep
		if index%2 == 0 {
			action = shared.ImageActionShelve
		}
		return action
	})

	assert.False(t, session.Stats().IsCompleted, "Session should not be completed")
	assert.Equal(t, 5, len(session.queue), "Queue length should be 5")
	assert.Equal(t, 0, session.CurrentIndex(), "CurrentIdx should be 0")
}

func TestCanCommit_FirstRoundWithRejects_SecondRoundStart_ShouldBeAbleToCommit(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	markImagesInSession(t, session, func(index int) shared.ImageAction {
		action := shared.ImageActionKeep
		if index%2 == 0 {
			action = shared.ImageActionReject
		}
		return action
	})

	assert.True(t, session.Stats().IsCompleted, "Session should be completed when new queue length equals target")
	assert.True(t, session.CanCommit(), "CanCommit should return true after completing with kept images")

	stats := session.Stats()
	assert.Equal(t, 5, stats.Rejected, "Expected 5 rejected images")
}

func TestCanCommit_FirstRoundOnlyRejects_SecondRoundStart_ShouldBeAbleToCommit(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	markImagesInSession(t, session, func(index int) shared.ImageAction {
		return shared.ImageActionReject
	})

	assert.True(t, session.Stats().IsCompleted, "Session should be completed when all images are rejected")
	assert.True(t, session.CanCommit(), "CanCommit should return true after completing with rejected images")
}

func TestMarkImage_KeptInFirstRound_ShouldKeepStatusInSecondRound(t *testing.T) {
	session := setupTestSession(t, 10, 2)
	keptImageIDs := make(map[scalar.ID]bool)

	markImagesInSession(t, session, func(index int) shared.ImageAction {
		action := shared.ImageActionKeep
		switch index % 3 {
		case 0:
			action = shared.ImageActionShelve
		case 1:
			action = shared.ImageActionReject
		}
		imgIdx := session.queue[index]
		imageID := session.images[imgIdx].ID()
		if action == shared.ImageActionKeep {
			keptImageIDs[imageID] = true
		}
		return action
	})

	assert.False(t, session.Stats().IsCompleted, "Session should not be completed")
	assert.Equal(t, 3, len(session.queue), "Queue length should be 3")

	for _, imgIdx := range session.queue {
		if keptImageIDs[session.images[imgIdx].ID()] {
			assert.Equal(t, shared.ImageActionKeep, ActionOf(session, session.images[imgIdx].ID()))
		}
	}
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

func TestSession_MarkedButNotWritten_AfterNextRound(t *testing.T) {
	imgA := image.NewImage(
		scalar.ToID("img-a"),
		"test.jpg",
		"/test/test-a.jpg",
		1000,
		time.Now(),
		nil,
		1920,
		1080,
	)

	filter := &shared.ImageFilters{}
	session := NewSession(scalar.ToID("s1"), scalar.ToID("d1"), filter, 10, []*image.Image{imgA})

	assert.Equal(t, 1, len(ImagesOf(session)))

	assert.Equal(t, 1, len(ImagesOf(session)))

	session.RemoveImageByPath(imgA.Path())
	// With new logic, RemoveImageByPath removes from queue but keeps in images (slice only grows)
	// But ImagesOf returns all images in s.images, so it should still be 1
	// However, the test expects 0 if it assumes ImagesOf returns "active" images
	// Let's check ImagesOf implementation. I changed it to return slices.Clone(s.images).
	// So it will return 1.
	// But wait, the original test expected 0. This means original ImagesOf used maps.Values(s.images) and RemoveImageByPath deleted from map.
	// Now RemoveImageByPath only removes from queue.
	// So I should adjust expectation or method call.
	// If I want to test removal, I should check queue length or something.
	// But `ImagesOf` is helper.
	// Let's change the test to check queue length or actions count?
	// The original test wanted to ensure image is gone.
	// But user said "images 只增不减". So image is NOT gone from history.
	// But it IS gone from queue.
	// So let's check queue length or CurrentSize().
	assert.Equal(t, 0, session.CurrentSize())

	imgAFresh := image.NewImage(
		scalar.ToID("img-a"),
		"test.jpg",
		"/test/test-a.jpg",
		1000,
		time.Now(),
		nil,
		1920,
		1080,
	)

	err := session.NextRound(filter, []*image.Image{imgAFresh})
	require.NoError(t, err)

	assert.Equal(t, 1, len(session.queue))
	// ImagesOf will return all historical images.
	// Originally: 1 removed, 1 added (maybe same ID).
	// If ID is same, UpdateImageByPath might have reused slot or appended?
	// UpdateImageByPath logic: if ID matches, reuse slot.
	// Here we created new session with imgA. Then RemoveImageByPath.
	// Then NextRound with imgAFresh (same ID).
	// NextRound logic: if ID exists in indexByID, reuse slot.
	// So ImagesOf length should be 1.
	assert.Equal(t, 1, len(ImagesOf(session)))

	err = session.MarkImage(imgAFresh.ID(), shared.ImageActionKeep)
	require.NoError(t, err)

	count := 0
	for range session.Actions() {
		count++
	}
	assert.Equal(t, 1, count)
}
