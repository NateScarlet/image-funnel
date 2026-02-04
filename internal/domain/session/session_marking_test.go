package session

import (
	"main/internal/apperror"
	"main/internal/scalar"
	"main/internal/shared"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurrentImage_ShouldReturnCorrectImage(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	currentImage := session.CurrentImage()
	assert.NotNil(t, currentImage, "CurrentImage should not be nil")
	assert.Equal(t, session.queue[0].ID(), currentImage.ID(), "CurrentImage ID should match")

	firstImageID := session.queue[0].ID()
	err := session.MarkImage(firstImageID, shared.ImageActionKeep)
	require.NoError(t, err)

	currentImage = session.CurrentImage()
	assert.NotNil(t, currentImage, "CurrentImage should not be nil")
	assert.NotEqual(t, firstImageID, currentImage.ID(), "CurrentImage ID should not match first image")
	assert.Equal(t, session.queue[1].ID(), currentImage.ID(), "CurrentImage ID should match second image")
}

func TestMarkImage_ShouldUpdateAction(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	imageID := session.queue[0].ID()
	err := session.MarkImage(imageID, shared.ImageActionKeep)
	require.NoError(t, err)

	assert.Equal(t, 1, session.CurrentIndex(), "CurrentIndex should be 1")
	assert.Equal(t, shared.ImageActionKeep, ActionOf(session, session.queue[0].ID()), "Action should be KEEP")
	assert.True(t, session.CanUndo(), "CanUndo should be true")
}

func TestMarkImage_NonCurrentImage_ShouldFindAndMark(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	imageID := session.queue[2].ID()
	err := session.MarkImage(imageID, shared.ImageActionShelve)
	require.NoError(t, err)

	assert.Equal(t, 3, session.CurrentIndex(), "CurrentIndex should be 3")
	assert.Equal(t, shared.ImageActionShelve, ActionOf(session, session.queue[2].ID()), "Action should be SHELVE")
}

func TestMarkImage_InvalidImageID_ShouldReturnError(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	err := session.MarkImage(scalar.ToID("invalid-id"), shared.ImageActionKeep)
	assert.Error(t, err, "Should return error for invalid image ID")
	assert.True(t, apperror.IsNotFound(err), "Error should be not found error")
}

func TestCanCommit_InitialState_ShouldReturnFalse(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	assert.False(t, session.CanCommit(), "CanCommit should return false for initial state")
}

func TestCanCommit_AfterMarkingImages_ShouldReturnTrue(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	for i := 0; i < 3; i++ {
		err := session.MarkImage(session.queue[i].ID(), shared.ImageActionKeep)
		require.NoError(t, err)
	}

	assert.True(t, session.CanCommit(), "CanCommit should return true after marking images")
}

func TestCanCommit_FirstRoundSingleReject_ShouldBeAbleToCommit(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	err := session.MarkImage(session.queue[0].ID(), shared.ImageActionReject)
	require.NoError(t, err)

	assert.False(t, session.Stats().IsCompleted, "Session should not be completed")
	assert.True(t, session.CanCommit(), "CanCommit should return true after rejecting one image in first round")

	stats := session.Stats()
	assert.Equal(t, 1, stats.Rejected, "Expected 1 rejected image")
}

func TestCanUndo_InitialState_ShouldReturnFalse(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	assert.False(t, session.CanUndo(), "CanUndo should return false for initial state")
}

func TestCanUndo_AfterMarkingImages_ShouldReturnTrue(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	err := session.MarkImage(session.queue[0].ID(), shared.ImageActionKeep)
	require.NoError(t, err)

	assert.True(t, session.CanUndo(), "CanUndo should return true after marking images")
}

func TestCanUndo_AfterUndoAll_ShouldReturnFalse(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	assert.False(t, session.CanUndo(), "CanUndo should return false when undoStack is empty")
}

func TestSession_MarkImage_OrderIndependence_And_Undo(t *testing.T) {
	session := setupTestSession(t, 5, 5)
	img0 := session.queue[0]
	img2 := session.queue[2]

	// 1. 正常顺序标记 Img0 -> Keep
	err := session.MarkImage(img0.ID(), shared.ImageActionKeep)
	require.NoError(t, err)
	assert.Equal(t, shared.ImageActionKeep, ActionOf(session, img0.ID()))
	assert.Equal(t, 1, session.CurrentIndex())

	// 2. 跳跃标记 Img2 -> Reject (跳过 Img1)
	err = session.MarkImage(img2.ID(), shared.ImageActionReject)
	require.NoError(t, err)
	assert.Equal(t, shared.ImageActionReject, ActionOf(session, img2.ID()))
	assert.Equal(t, 3, session.CurrentIndex()) // Index should move to 2+1=3

	// 3. 回头修改 Img0 -> Shelve (修改已有状态)
	err = session.MarkImage(img0.ID(), shared.ImageActionShelve)
	require.NoError(t, err)
	assert.Equal(t, shared.ImageActionShelve, ActionOf(session, img0.ID()))
	assert.Equal(t, 1, session.CurrentIndex()) // Back to 0+1=1

	// 4. 重复标记 Img0 -> Shelve (幂等操作，但会推入撤销栈)
	err = session.MarkImage(img0.ID(), shared.ImageActionShelve)
	require.NoError(t, err)
	assert.Equal(t, shared.ImageActionShelve, ActionOf(session, img0.ID()))

	// 开始撤销验证

	// 撤销 4 (重复标记)
	err = session.Undo()
	require.NoError(t, err)
	assert.Equal(t, shared.ImageActionShelve, ActionOf(session, img0.ID()))

	// 撤销 3 (修改 Img0: Keep -> Shelve)
	err = session.Undo()
	require.NoError(t, err)
	assert.Equal(t, shared.ImageActionKeep, ActionOf(session, img0.ID()))
	assert.Equal(t, shared.ImageActionReject, ActionOf(session, img2.ID()))

	// 撤销 2 (跳跃标记 Img2)
	err = session.Undo()
	require.NoError(t, err)
	assert.True(t, ActionOf(session, img2.ID()).IsZero())
	assert.Equal(t, shared.ImageActionKeep, ActionOf(session, img0.ID()))

	// 撤销 1 (初始标记 Img0)
	err = session.Undo()
	require.NoError(t, err)
	assert.True(t, ActionOf(session, img0.ID()).IsZero())
}
