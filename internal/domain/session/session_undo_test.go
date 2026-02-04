package session

import (
	"main/internal/domain/image"
	"main/internal/shared"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUndo_ShouldRestorePreviousAction(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	imageID := session.queue[0].ID()
	err := session.MarkImage(imageID, shared.ImageActionKeep)
	require.NoError(t, err)

	assert.Equal(t, 1, session.CurrentIndex(), "CurrentIndex should be 1")

	err = session.Undo()
	require.NoError(t, err)

	assert.Equal(t, 0, session.CurrentIndex(), "CurrentIndex should be 0")
	assert.True(t, ActionOf(session, session.queue[0].ID()).IsZero(), "Action should be restored to zero (Pending)")
	assert.False(t, session.CanUndo(), "CanUndo should be false after undo")
}

func TestUndo_NothingToUndo_ShouldReturnError(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	err := session.Undo()
	assert.Error(t, err, "Should return error when nothing to undo")
	assert.Equal(t, ErrNothingToUndo, err, "Error should be ErrNothingToUndo")
}

func TestUndo_ShouldRestoreActiveStatus(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	markImagesInSession(t, session, func(index int) shared.ImageAction {
		return shared.ImageActionReject
	})

	assert.True(t, session.Stats().IsCompleted(), "IsCompleted should be true")

	err := session.Undo()
	require.NoError(t, err)

	assert.False(t, session.Stats().IsCompleted(), "IsCompleted should be false after undo")
}

func TestCanUndo_AfterRoundCompletion_ShouldAllowCrossRoundUndo(t *testing.T) {
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

	assert.False(t, session.Stats().IsCompleted(), "Session should not be completed for second round")
	assert.True(t, session.CanUndo(), "CanUndo should return true after round completion for cross-round undo")

	err := session.Undo()
	require.NoError(t, err)

	assert.False(t, session.Stats().IsCompleted(), "Session should not be completed after cross-round undo")
	assert.Equal(t, 10, len(session.queue), "Queue length should be restored to original 10")
	assert.Equal(t, 9, session.CurrentIndex(), "CurrentIndex should be restored to last processed index (9) after cross-round undo")
}

func TestUndo_ShouldRestoreToPreviousRound(t *testing.T) {
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

	assert.False(t, session.Stats().IsCompleted(), "Session should not be completed after first round")
	assert.Equal(t, 3, len(session.queue), "Queue should have 3 images for second round")
	assert.Equal(t, 0, session.CurrentIndex(), "CurrentIndex should be 0 for second round")

	err := session.MarkImage(session.queue[0].ID(), shared.ImageActionKeep)
	require.NoError(t, err)

	err = session.Undo()
	require.NoError(t, err)

	assert.Equal(t, shared.ImageActionKeep, ActionOf(session, session.queue[0].ID()), "Action should be restored to KEEP (from previous round) after undo in second round")
	assert.Equal(t, 0, session.CurrentIndex(), "CurrentIndex should be 0 after undo")
}

func TestUndo_ShouldRestoreToPreviousRoundWhenUndoStackEmpty(t *testing.T) {
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

	assert.False(t, session.Stats().IsCompleted(), "Session should not be completed after first round")
	assert.Equal(t, 3, len(session.queue), "Queue should have 3 images for second round")
	assert.Equal(t, 1, session.currentRound, "CurrentRound should be 1")

	err := session.Undo()
	require.NoError(t, err)

	assert.Equal(t, 0, session.currentRound, "CurrentRound should be 0 after undo to previous round")
	assert.Equal(t, 10, len(session.queue), "Queue should be restored to 10 images")
	assert.Equal(t, 9, session.CurrentIndex(), "CurrentIndex should be 9 after undo to previous round")
}

func TestUndo_ShouldHandleNoMoreUndoActions(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	markImagesInSession(t, session, func(index int) shared.ImageAction {
		return shared.ImageActionReject
	})

	assert.True(t, session.Stats().IsCompleted(), "Session should be completed")

	err := session.Undo()
	require.NoError(t, err)

	assert.False(t, session.Stats().IsCompleted(), "Session should not be completed after undo")
	assert.True(t, ActionOf(session, session.queue[9].ID()).IsZero(), "Last image action should be restored to zero (Pending)")
}

func TestUndo_CrossRoundToBeginning(t *testing.T) {
	session := setupTestSession(t, 2, 1)

	img0ID := session.queue[0].ID()
	img1ID := session.queue[1].ID()

	err := session.MarkImage(img0ID, shared.ImageActionKeep)
	require.NoError(t, err)

	err = session.MarkImage(img1ID, shared.ImageActionKeep)
	require.NoError(t, err)

	require.Equal(t, 1, session.currentRound)

	err = session.Undo()
	require.NoError(t, err)
	assert.Equal(t, 0, session.currentRound)
	assert.Equal(t, 1, session.currentIdx)
	assert.True(t, ActionOf(session, img1ID).IsZero())

	err = session.Undo()
	require.NoError(t, err)
	assert.Equal(t, 0, session.currentIdx)
	assert.True(t, ActionOf(session, img0ID).IsZero())

	err = session.Undo()
	assert.Equal(t, ErrNothingToUndo, err)
}

func TestUndo_ShouldRestoreIndex_WhenRemarking(t *testing.T) {
	session := setupTestSession(t, 2, 1)
	img0ID := session.queue[0].ID()

	err := session.MarkImage(img0ID, shared.ImageActionKeep)
	require.NoError(t, err)
	assert.Equal(t, 1, session.currentIdx)

	err = session.MarkImage(img0ID, shared.ImageActionKeep)
	require.NoError(t, err)
	assert.Equal(t, 1, session.currentIdx)

	err = session.Undo()
	require.NoError(t, err)
	assert.Equal(t, 0, session.currentIdx)

	err = session.Undo()
	require.NoError(t, err)
	assert.Equal(t, 0, session.currentIdx)

	assert.True(t, session.currentIdx >= 0)
}

func TestUndo_AfterUpdateAndNextRound(t *testing.T) {
	session := setupTestSession(t, 5, 2)
	assert.Equal(t, 0, session.currentRound)

	err := session.MarkImage(session.queue[0].ID(), shared.ImageActionKeep)
	require.NoError(t, err)

	err = session.MarkImage(session.queue[1].ID(), shared.ImageActionKeep)
	require.NoError(t, err)

	assert.Equal(t, 2, session.currentIdx)

	err = session.NextRound(nil, session.queue)
	require.NoError(t, err)

	assert.Equal(t, 1, session.currentRound)
	assert.Equal(t, 0, session.currentIdx)

	err = session.Undo()
	require.NoError(t, err)

	assert.Equal(t, 0, session.currentRound)
	assert.Equal(t, 2, session.currentIdx, "CurrentIdx should be restored to 2")
}

func TestUndo_ShouldRestoreFilter_WhenUndoNextRound(t *testing.T) {
	session := setupTestSession(t, 10, 5)
	initialFilter := session.Filter()

	err := session.MarkImage(session.queue[0].ID(), shared.ImageActionKeep)
	require.NoError(t, err)

	newFilter := &shared.ImageFilters{Rating: []int{5}}
	err = session.NextRound(newFilter, []*image.Image{})
	require.NoError(t, err)

	assert.Equal(t, newFilter, session.Filter())

	err = session.Undo()
	require.NoError(t, err)

	assert.Equal(t, initialFilter, session.Filter(), "Filter should be restored to initial filter")
}
