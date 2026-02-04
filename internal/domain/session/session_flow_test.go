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

	assert.True(t, session.Stats().IsCompleted(), "IsCompleted should be true")
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

	assert.False(t, session.Stats().IsCompleted(), "Session should not be completed")

	newRoundStats := session.Stats()
	assert.Equal(t, 0, session.CurrentIndex(), "New round processed should be 0")
	assert.Equal(t, 3, newRoundStats.Total(), "New round total should be 3")
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

	assert.False(t, session.Stats().IsCompleted(), "Session should not be completed")
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

	assert.True(t, session.Stats().IsCompleted(), "Session should be completed when new queue length equals target")
	assert.True(t, session.CanCommit(), "CanCommit should return true after completing with kept images")

	stats := session.Stats()
	assert.Equal(t, 5, stats.Rejected(), "Expected 5 rejected images")
}

func TestCanCommit_FirstRoundOnlyRejects_SecondRoundStart_ShouldBeAbleToCommit(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	markImagesInSession(t, session, func(index int) shared.ImageAction {
		return shared.ImageActionReject
	})

	assert.True(t, session.Stats().IsCompleted(), "Session should be completed when all images are rejected")
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
		imageID := session.queue[index].ID()
		if action == shared.ImageActionKeep {
			keptImageIDs[imageID] = true
		}
		return action
	})

	assert.False(t, session.Stats().IsCompleted(), "Session should not be completed")
	assert.Equal(t, 3, len(session.queue), "Queue length should be 3")

	for _, img := range session.queue {
		if keptImageIDs[img.ID()] {
			assert.Equal(t, shared.ImageActionKeep, ActionOf(session, img.ID()))
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

	assert.True(t, session.Stats().IsCompleted(), "Session should be completed when kept <= target")
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

	assert.True(t, session.Stats().IsCompleted(), "Session should be completed when kept == target")
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

	session.RemoveImageByPath(imgA.Path())
	assert.Equal(t, 0, len(ImagesOf(session)))

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
	assert.Equal(t, 1, len(ImagesOf(session)))

	err = session.MarkImage(imgAFresh.ID(), shared.ImageActionKeep)
	require.NoError(t, err)

	count := 0
	for range session.Actions() {
		count++
	}
	assert.Equal(t, 1, count)
}
