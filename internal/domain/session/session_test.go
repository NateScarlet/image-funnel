package session

import (
	"fmt"
	"main/internal/scalar"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSession_ShouldInitializeCorrectly(t *testing.T) {
	filter := NewImageFilters([]int{0, 1, 2})
	images := createTestImages(10)

	session := NewSession("/test", filter, 5, images)

	assert.NotEmpty(t, session.ID(), "Session ID should not be empty")
	assert.Equal(t, "/test", session.Directory(), "Directory should match")
	assert.Equal(t, filter, session.Filter(), "Filter should match")
	assert.Equal(t, 5, session.TargetKeep(), "TargetKeep should match")
	assert.Equal(t, StatusActive, session.Status(), "Status should be ACTIVE")
	assert.Equal(t, 10, len(session.Images()), "Images count should match")
	assert.Equal(t, 10, len(session.queue), "Queue count should match")
	assert.Equal(t, 0, session.CurrentIndex(), "CurrentIndex should be 0")
	assert.False(t, session.CanUndo(), "CanUndo should be false initially")
}

func TestStats_InitialState(t *testing.T) {
	filter := NewImageFilters([]int{0})
	images := createTestImages(10)

	session := NewSession("/test", filter, 5, images)

	stats := session.Stats()

	assert.Equal(t, 10, stats.Total(), "Total should be 10")
	assert.Equal(t, 0, stats.Processed(), "Processed should be 0")
	assert.Equal(t, 0, stats.Kept(), "Kept should be 0")
	assert.Equal(t, 0, stats.Reviewed(), "Reviewed should be 0")
	assert.Equal(t, 0, stats.Rejected(), "Rejected should be 0")
	assert.Equal(t, 10, stats.Remaining(), "Remaining should be 10")
}

func TestStats_AfterMarkingImages(t *testing.T) {
	filter := NewImageFilters([]int{0})
	images := createTestImages(10)

	session := NewSession("/test", filter, 5, images)

	for i := 0; i < 3; i++ {
		err := session.MarkImage(session.queue[i].ID(), ActionKeep)
		require.NoError(t, err)
	}

	for i := 3; i < 6; i++ {
		err := session.MarkImage(session.queue[i].ID(), ActionPending)
		require.NoError(t, err)
	}

	for i := 6; i < 9; i++ {
		err := session.MarkImage(session.queue[i].ID(), ActionReject)
		require.NoError(t, err)
	}

	stats := session.Stats()

	assert.Equal(t, 10, stats.Total(), "Total should be 10")
	assert.Equal(t, 9, stats.Processed(), "Processed should be 9")
	assert.Equal(t, 3, stats.Kept(), "Kept should be 3")
	assert.Equal(t, 3, stats.Reviewed(), "Reviewed should be 3")
	assert.Equal(t, 3, stats.Rejected(), "Rejected should be 3")
	assert.Equal(t, 1, stats.Remaining(), "Remaining should be 1")
}

func TestCurrentImage_ShouldReturnCorrectImage(t *testing.T) {
	filter := NewImageFilters([]int{0})
	images := createTestImages(10)

	session := NewSession("/test", filter, 5, images)

	currentImage := session.CurrentImage()
	assert.NotNil(t, currentImage, "CurrentImage should not be nil")
	assert.Equal(t, images[0].ID(), currentImage.ID(), "CurrentImage ID should match")

	err := session.MarkImage(session.queue[0].ID(), ActionKeep)
	require.NoError(t, err)

	currentImage = session.CurrentImage()
	assert.NotNil(t, currentImage, "CurrentImage should not be nil")
	assert.Equal(t, images[1].ID(), currentImage.ID(), "CurrentImage ID should match second image")
}

func TestMarkImage_ShouldUpdateAction(t *testing.T) {
	filter := NewImageFilters([]int{0})
	images := createTestImages(10)

	session := NewSession("/test", filter, 5, images)

	imageID := session.queue[0].ID()
	err := session.MarkImage(imageID, ActionKeep)
	require.NoError(t, err)

	assert.Equal(t, 1, session.CurrentIndex(), "CurrentIndex should be 1")
	assert.Equal(t, ActionKeep, session.queue[0].Action(), "Action should be KEEP")
	assert.True(t, session.CanUndo(), "CanUndo should be true")
}

func TestMarkImage_NonCurrentImage_ShouldFindAndMark(t *testing.T) {
	filter := NewImageFilters([]int{0})
	images := createTestImages(10)

	session := NewSession("/test", filter, 5, images)

	imageID := session.queue[2].ID()
	err := session.MarkImage(imageID, ActionPending)
	require.NoError(t, err)

	assert.Equal(t, 3, session.CurrentIndex(), "CurrentIndex should be 3")
	assert.Equal(t, ActionPending, session.queue[2].Action(), "Action should be PENDING")
}

func TestMarkImage_InvalidImageID_ShouldReturnError(t *testing.T) {
	filter := NewImageFilters([]int{0})
	images := createTestImages(10)

	session := NewSession("/test", filter, 5, images)

	err := session.MarkImage(scalar.ToID("invalid-id"), ActionKeep)
	assert.Error(t, err, "Should return error for invalid image ID")
	assert.Equal(t, ErrSessionNotFound, err, "Error should be ErrSessionNotFound")
}

func TestMarkImage_SessionNotActive_ShouldReturnError(t *testing.T) {
	filter := NewImageFilters([]int{0})
	images := createTestImages(10)

	session := NewSession("/test", filter, 5, images)
	session.status = StatusCompleted

	err := session.MarkImage(session.queue[0].ID(), ActionKeep)
	assert.Error(t, err, "Should return error when session is not active")
	assert.Equal(t, ErrSessionNotActive, err, "Error should be ErrSessionNotActive")
}

func TestMarkImage_AllImagesRejected_ShouldCompleteSession(t *testing.T) {
	filter := NewImageFilters([]int{0})
	images := createTestImages(10)

	session := NewSession("/test", filter, 5, images)

	for i := 0; i < 10; i++ {
		err := session.MarkImage(session.queue[i].ID(), ActionReject)
		require.NoError(t, err)
	}

	assert.Equal(t, StatusCompleted, session.Status(), "Status should be COMPLETED")
}

func TestMarkImage_KeepAndReview_ShouldStartNextRound(t *testing.T) {
	filter := NewImageFilters([]int{0})
	images := createTestImages(10)

	session := NewSession("/test", filter, 5, images)

	for i := 0; i < 10; i++ {
		action := ActionKeep
		if i%3 == 0 {
			action = ActionPending
		} else if i%3 == 1 {
			action = ActionReject
		}
		err := session.MarkImage(session.queue[i].ID(), action)
		require.NoError(t, err)
	}

	assert.Equal(t, StatusActive, session.Status(), "Session status should be ACTIVE")

	newRoundStats := session.Stats()
	assert.Equal(t, 0, newRoundStats.Processed(), "New round processed should be 0")
	assert.Equal(t, 7, newRoundStats.Total(), "New round total should be 7")
	assert.Equal(t, 7, newRoundStats.Remaining(), "New round remaining should be 7")

	expectedQueueLength := 7
	assert.Equal(t, expectedQueueLength, len(session.queue), "Queue length should be %d", expectedQueueLength)
	assert.Equal(t, 0, session.CurrentIndex(), "CurrentIdx should be 0")
}

func TestMarkImage_KeepAndReview_ShouldStartNextRoundWithBoth(t *testing.T) {
	filter := NewImageFilters([]int{0})
	images := createTestImages(10)

	session := NewSession("/test", filter, 5, images)

	for i := 0; i < 10; i++ {
		action := ActionKeep
		if i%2 == 0 {
			action = ActionPending
		}
		err := session.MarkImage(session.queue[i].ID(), action)
		require.NoError(t, err)
	}

	assert.Equal(t, StatusActive, session.Status(), "Session status should be ACTIVE")

	expectedQueueLength := 10
	assert.Equal(t, expectedQueueLength, len(session.queue), "Queue length should be %d", expectedQueueLength)
	assert.Equal(t, 0, session.CurrentIndex(), "CurrentIdx should be 0")
}

func TestCanCommit_InitialState_ShouldReturnFalse(t *testing.T) {
	filter := NewImageFilters([]int{0})
	images := createTestImages(10)

	session := NewSession("/test", filter, 5, images)

	assert.False(t, session.CanCommit(), "CanCommit should return false for initial state")
}

func TestCanCommit_AfterMarkingImages_ShouldReturnTrue(t *testing.T) {
	filter := NewImageFilters([]int{0})
	images := createTestImages(10)

	session := NewSession("/test", filter, 5, images)

	for i := 0; i < 3; i++ {
		err := session.MarkImage(session.queue[i].ID(), ActionKeep)
		require.NoError(t, err)
	}

	assert.True(t, session.CanCommit(), "CanCommit should return true after marking images")
}

func TestCanCommit_CommittingStatus_ShouldReturnFalse(t *testing.T) {
	filter := NewImageFilters([]int{0})
	images := createTestImages(10)

	session := NewSession("/test", filter, 5, images)
	session.status = StatusCommitting

	assert.False(t, session.CanCommit(), "CanCommit should return false for COMMITTING status")
}

func TestCanCommit_ErrorStatus_ShouldReturnFalse(t *testing.T) {
	filter := NewImageFilters([]int{0})
	images := createTestImages(10)

	session := NewSession("/test", filter, 5, images)
	session.status = StatusError

	assert.False(t, session.CanCommit(), "CanCommit should return false for ERROR status")
}

func TestCanUndo_InitialState_ShouldReturnFalse(t *testing.T) {
	filter := NewImageFilters([]int{0})
	images := createTestImages(10)

	session := NewSession("/test", filter, 5, images)

	assert.False(t, session.CanUndo(), "CanUndo should return false for initial state")
}

func TestCanUndo_AfterMarkingImages_ShouldReturnTrue(t *testing.T) {
	filter := NewImageFilters([]int{0})
	images := createTestImages(10)

	session := NewSession("/test", filter, 5, images)

	err := session.MarkImage(session.queue[0].ID(), ActionKeep)
	require.NoError(t, err)

	assert.True(t, session.CanUndo(), "CanUndo should return true after marking images")
}

func TestCanUndo_AfterUndoAll_ShouldReturnFalse(t *testing.T) {
	filter := NewImageFilters([]int{0})
	images := createTestImages(10)

	session := NewSession("/test", filter, 5, images)

	assert.False(t, session.CanUndo(), "CanUndo should return false when undoStack is empty")
}

func TestCanCommit_FirstRoundWithRejects_SecondRoundStart_ShouldBeAbleToCommit(t *testing.T) {
	filter := NewImageFilters([]int{0})
	images := createTestImages(10)

	session := NewSession("/test", filter, 5, images)

	for i := 0; i < 10; i++ {
		action := ActionKeep
		if i%2 == 0 {
			action = ActionReject
		}
		err := session.MarkImage(session.queue[i].ID(), action)
		require.NoError(t, err)
	}

	assert.Equal(t, StatusCompleted, session.Status(), "Session status should be COMPLETED when new queue length equals target")
	assert.True(t, session.CanCommit(), "CanCommit should return true after completing with kept images")

	stats := session.Stats()
	assert.Equal(t, 5, stats.Rejected(), "Expected 5 rejected images")
}

func TestCanCommit_FirstRoundOnlyRejects_SecondRoundStart_ShouldBeAbleToCommit(t *testing.T) {
	filter := NewImageFilters([]int{0})
	images := createTestImages(10)

	session := NewSession("/test", filter, 5, images)

	for i := 0; i < 10; i++ {
		err := session.MarkImage(session.queue[i].ID(), ActionReject)
		require.NoError(t, err)
	}

	assert.Equal(t, StatusCompleted, session.Status(), "Session status should be COMPLETED when all images are rejected")
	assert.True(t, session.CanCommit(), "CanCommit should return true after completing with rejected images")
}

func TestCanCommit_FirstRoundSingleReject_ShouldBeAbleToCommit(t *testing.T) {
	filter := NewImageFilters([]int{0})
	images := createTestImages(10)

	session := NewSession("/test", filter, 5, images)

	err := session.MarkImage(session.queue[0].ID(), ActionReject)
	require.NoError(t, err)

	assert.Equal(t, StatusActive, session.Status(), "Session status should be ACTIVE")
	assert.True(t, session.CanCommit(), "CanCommit should return true after rejecting one image in first round")

	stats := session.Stats()
	assert.Equal(t, 1, stats.Rejected(), "Expected 1 rejected image")
}

func TestMarkImage_KeptInFirstRound_ShouldKeepStatusInSecondRound(t *testing.T) {
	filter := NewImageFilters([]int{0})
	images := createTestImages(10)

	session := NewSession("/test", filter, 5, images)

	keptImageIDs := make(map[scalar.ID]bool)

	for i := 0; i < 10; i++ {
		action := ActionKeep
		if i%3 == 0 {
			action = ActionPending
		} else if i%3 == 1 {
			action = ActionReject
		}
		imageID := session.queue[i].ID()
		if action == ActionKeep {
			keptImageIDs[imageID] = true
		}
		err := session.MarkImage(imageID, action)
		require.NoError(t, err)
	}

	assert.Equal(t, StatusActive, session.Status(), "Session status should be ACTIVE")

	expectedQueueLength := 7
	assert.Equal(t, expectedQueueLength, len(session.queue), "Queue length should be %d", expectedQueueLength)
	assert.Equal(t, 0, session.CurrentIndex(), "CurrentIdx should be 0")

	for _, img := range session.queue {
		if keptImageIDs[img.ID()] {
			assert.Equal(t, ActionKeep, img.Action(), "Image %s was marked as KEEP in first round, but action is %s in second round", img.ID(), img.Action())
		}
	}
}

func TestUndo_ShouldRestorePreviousAction(t *testing.T) {
	filter := NewImageFilters([]int{0})
	images := createTestImages(10)

	session := NewSession("/test", filter, 5, images)

	imageID := session.queue[0].ID()
	err := session.MarkImage(imageID, ActionKeep)
	require.NoError(t, err)

	assert.Equal(t, 1, session.CurrentIndex(), "CurrentIndex should be 1")

	err = session.Undo()
	require.NoError(t, err)

	assert.Equal(t, 0, session.CurrentIndex(), "CurrentIndex should be 0")
	assert.Equal(t, ActionPending, session.queue[0].Action(), "Action should be restored to PENDING")
	assert.False(t, session.CanUndo(), "CanUndo should be false after undo")
}

func TestUndo_NothingToUndo_ShouldReturnError(t *testing.T) {
	filter := NewImageFilters([]int{0})
	images := createTestImages(10)

	session := NewSession("/test", filter, 5, images)

	err := session.Undo()
	assert.Error(t, err, "Should return error when nothing to undo")
	assert.Equal(t, ErrNothingToUndo, err, "Error should be ErrNothingToUndo")
}

func TestUndo_ShouldRestoreActiveStatus(t *testing.T) {
	filter := NewImageFilters([]int{0})
	images := createTestImages(10)

	session := NewSession("/test", filter, 5, images)

	for i := 0; i < 10; i++ {
		err := session.MarkImage(session.queue[i].ID(), ActionReject)
		require.NoError(t, err)
	}

	assert.Equal(t, StatusCompleted, session.Status(), "Status should be COMPLETED")

	err := session.Undo()
	require.NoError(t, err)

	assert.Equal(t, StatusActive, session.Status(), "Status should be restored to ACTIVE")
}

func TestImage_Action_ShouldDefaultToPending(t *testing.T) {
	img := NewImage(scalar.ToID("test-id"), "test.jpg", "/test/test.jpg", 1000, time.Now(), 0, false)

	assert.Equal(t, ActionPending, img.Action(), "Action should default to PENDING")
}

func TestImage_SetAction_ShouldUpdateAction(t *testing.T) {
	img := NewImage(scalar.ToID("test-id"), "test.jpg", "/test/test.jpg", 1000, time.Now(), 0, false)

	img.SetAction(ActionKeep)
	assert.Equal(t, ActionKeep, img.Action(), "Action should be KEEP")

	img.SetAction(ActionReject)
	assert.Equal(t, ActionReject, img.Action(), "Action should be REJECT")
}

func TestImageFilters_Rating(t *testing.T) {
	filter := NewImageFilters([]int{0, 1, 2})

	assert.Equal(t, []int{0, 1, 2}, filter.Rating(), "Rating should match")
}

func TestImageFilters_Nil_ShouldReturnNil(t *testing.T) {
	var filter *ImageFilters

	assert.Nil(t, filter.Rating(), "Rating should be nil when filter is nil")
}

func TestWriteActions_Getters(t *testing.T) {
	actions := NewWriteActions(5, 3, 1)

	assert.Equal(t, 5, actions.KeepRating(), "KeepRating should match")
	assert.Equal(t, 3, actions.PendingRating(), "PendingRating should match")
	assert.Equal(t, 1, actions.RejectRating(), "RejectRating should match")
}

func TestStats_Getters(t *testing.T) {
	stats := &Stats{
		total:     10,
		processed: 5,
		kept:      2,
		reviewed:  2,
		rejected:  1,
		remaining: 5,
	}

	assert.Equal(t, 10, stats.Total(), "Total should match")
	assert.Equal(t, 5, stats.Processed(), "Processed should match")
	assert.Equal(t, 2, stats.Kept(), "Kept should match")
	assert.Equal(t, 2, stats.Reviewed(), "Reviewed should match")
	assert.Equal(t, 1, stats.Rejected(), "Rejected should match")
	assert.Equal(t, 5, stats.Remaining(), "Remaining should match")
}

func TestSessionError_Error(t *testing.T) {
	err := &SessionError{message: "test error"}

	assert.Equal(t, "test error", err.Error(), "Error message should match")
}

func createTestImages(count int) []*Image {
	images := make([]*Image, count)
	for i := 0; i < count; i++ {
		images[i] = NewImage(
			scalar.ToID(fmt.Sprintf("img-%d", i)),
			"test.jpg",
			fmt.Sprintf("/test/test-%d.jpg", i),
			1000,
			time.Now(),
			0,
			false,
		)
	}
	return images
}

func createTestImagesWithRatings(ratings []int) []*Image {
	images := make([]*Image, len(ratings))
	for i, rating := range ratings {
		images[i] = NewImage(
			scalar.ToID(fmt.Sprintf("img-%d", i)),
			"test.jpg",
			fmt.Sprintf("/test/test-%d.jpg", i),
			1000,
			time.Now(),
			rating,
			false,
		)
	}
	return images
}

func TestBuildImageFilter_WithRating(t *testing.T) {
	images := createTestImagesWithRatings([]int{0, 1, 2, 3, 4, 0, 1, 2, 3, 4})

	filter := NewImageFilters([]int{0, 1})
	filterFunc := BuildImageFilter(filter)
	filtered := FilterImages(images, filterFunc)

	assert.Equal(t, 4, len(filtered), "Should filter to 4 images with rating 0 or 1")
	for _, img := range filtered {
		assert.Contains(t, []int{0, 1}, img.Rating(), "Image rating should be 0 or 1")
	}
}

func TestBuildImageFilter_WithNilFilter(t *testing.T) {
	images := createTestImagesWithRatings([]int{0, 1, 2, 3, 4, 5})

	filterFunc := BuildImageFilter(nil)
	filtered := FilterImages(images, filterFunc)

	assert.Equal(t, 6, len(filtered), "Should include all images when filter is nil")
}

func TestBuildImageFilter_WithEmptyRating(t *testing.T) {
	images := createTestImagesWithRatings([]int{0, 1, 2, 3, 4, 5})

	filter := NewImageFilters([]int{})
	filterFunc := BuildImageFilter(filter)
	filtered := FilterImages(images, filterFunc)

	assert.Equal(t, 6, len(filtered), "Should include all images when rating is empty")
}

func TestBuildImageFilter_WithSingleRating(t *testing.T) {
	images := createTestImagesWithRatings([]int{0, 1, 2, 3, 4, 5, 2, 2})

	filter := NewImageFilters([]int{2})
	filterFunc := BuildImageFilter(filter)
	filtered := FilterImages(images, filterFunc)

	assert.Equal(t, 3, len(filtered), "Should filter to 3 images with rating 2")
	for _, img := range filtered {
		assert.Equal(t, 2, img.Rating(), "All images should have rating 2")
	}
}

func TestFilterImages_WithNilFilter(t *testing.T) {
	images := createTestImagesWithRatings([]int{0, 1, 2, 3, 4, 5})

	filtered := FilterImages(images, nil)

	assert.Equal(t, 6, len(filtered), "Should return all images when filter is nil")
}

func TestUndo_ShouldRestoreToPreviousRound(t *testing.T) {
	filter := NewImageFilters([]int{0})
	images := createTestImages(10)

	session := NewSession("/test", filter, 5, images)

	for i := 0; i < 10; i++ {
		action := ActionKeep
		if i%3 == 0 {
			action = ActionPending
		} else if i%3 == 1 {
			action = ActionReject
		}
		err := session.MarkImage(session.queue[i].ID(), action)
		require.NoError(t, err)
	}

	assert.Equal(t, StatusActive, session.Status(), "Session status should be ACTIVE after first round")
	assert.Equal(t, 7, len(session.queue), "Queue should have 7 images for second round")
	assert.Equal(t, 0, session.CurrentIndex(), "CurrentIndex should be 0 for second round")

	err := session.MarkImage(session.queue[0].ID(), ActionKeep)
	require.NoError(t, err)

	err = session.Undo()
	require.NoError(t, err)

	assert.Equal(t, ActionPending, session.queue[0].Action(), "Action should be restored to PENDING after undo in second round")
	assert.Equal(t, 0, session.CurrentIndex(), "CurrentIndex should be 0 after undo")
}

func TestUndo_ShouldRestoreToPreviousRoundWhenUndoStackEmpty(t *testing.T) {
	filter := NewImageFilters([]int{0})
	images := createTestImages(10)

	session := NewSession("/test", filter, 5, images)

	for i := 0; i < 10; i++ {
		action := ActionKeep
		if i%3 == 0 {
			action = ActionPending
		} else if i%3 == 1 {
			action = ActionReject
		}
		err := session.MarkImage(session.queue[i].ID(), action)
		require.NoError(t, err)
	}

	assert.Equal(t, StatusActive, session.Status(), "Session status should be ACTIVE after first round")
	assert.Equal(t, 7, len(session.queue), "Queue should have 7 images for second round")
	assert.Equal(t, 1, session.currentRound, "CurrentRound should be 1")

	err := session.Undo()
	require.NoError(t, err)

	assert.Equal(t, 0, session.currentRound, "CurrentRound should be 0 after undo to previous round")
	assert.Equal(t, 10, len(session.queue), "Queue should be restored to 10 images")
	assert.Equal(t, 10, session.CurrentIndex(), "CurrentIndex should be 10 after undo to previous round")
	assert.Equal(t, StatusActive, session.Status(), "Session status should be ACTIVE after undo")
}

func TestMarkImage_KeptLessOrEqualTarget_ShouldComplete(t *testing.T) {
	filter := NewImageFilters([]int{0})
	images := createTestImages(10)

	session := NewSession("/test", filter, 5, images)

	for i := 0; i < 10; i++ {
		action := ActionKeep
		if i >= 5 {
			action = ActionReject
		}
		err := session.MarkImage(session.queue[i].ID(), action)
		require.NoError(t, err)
	}

	assert.Equal(t, StatusCompleted, session.Status(), "Session should be COMPLETED when kept <= target")
}

func TestMarkImage_KeptEqualTarget_ShouldComplete(t *testing.T) {
	filter := NewImageFilters([]int{0})
	images := createTestImages(10)

	session := NewSession("/test", filter, 5, images)

	for i := 0; i < 10; i++ {
		action := ActionKeep
		if i >= 5 {
			action = ActionReject
		}
		err := session.MarkImage(session.queue[i].ID(), action)
		require.NoError(t, err)
	}

	assert.Equal(t, StatusCompleted, session.Status(), "Session should be COMPLETED when kept == target")
}

func TestUndo_ShouldWorkAfterCompletion(t *testing.T) {
	filter := NewImageFilters([]int{0})
	images := createTestImages(10)

	session := NewSession("/test", filter, 5, images)

	for i := 0; i < 10; i++ {
		err := session.MarkImage(session.queue[i].ID(), ActionReject)
		require.NoError(t, err)
	}

	assert.Equal(t, StatusCompleted, session.Status(), "Session should be COMPLETED")

	err := session.Undo()
	require.NoError(t, err)

	assert.Equal(t, StatusActive, session.Status(), "Session should be ACTIVE after undo")
	assert.Equal(t, ActionPending, session.queue[9].Action(), "Last image action should be restored to PENDING")
}
