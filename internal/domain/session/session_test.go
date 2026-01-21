package session

import (
	"fmt"
	"main/internal/domain/image"
	"main/internal/domain/metadata"
	"main/internal/scalar"
	"main/internal/shared"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSession_ShouldInitializeCorrectly(t *testing.T) {
	filter := &shared.ImageFilters{Rating: []int{0, 1, 2}}
	images := createTestImages(10)

	session := NewSession(scalar.ToID("test-id"), scalar.ToID("test-dir-id"), filter, 5, images)

	assert.NotEmpty(t, session.ID(), "Session ID should not be empty")
	assert.Equal(t, scalar.ToID("test-dir-id"), session.DirectoryID(), "DirectoryID should match")
	assert.Equal(t, filter, session.Filter(), "Filter should match")
	assert.Equal(t, 5, session.TargetKeep(), "TargetKeep should match")
	assert.False(t, session.Stats().IsCompleted(), "IsCompleted should be false initially")
	assert.Equal(t, 10, len(session.Images()), "Images count should match")
	assert.Equal(t, 10, len(session.queue), "Queue count should match")
	assert.Equal(t, 0, session.CurrentIndex(), "CurrentIndex should be 0")
	assert.False(t, session.CanUndo(), "CanUndo should be false initially")
}

func TestStats_InitialState(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	stats := session.Stats()

	assert.Equal(t, 10, stats.Total(), "Total should be 10")
	assert.Equal(t, 0, session.CurrentIndex(), "Processed should be 0")
	assert.Equal(t, 0, stats.Kept(), "Kept should be 0")
	assert.Equal(t, 0, stats.Reviewed(), "Reviewed should be 0")
	assert.Equal(t, 0, stats.Rejected(), "Rejected should be 0")
	assert.Equal(t, 10, stats.Remaining(), "Remaining should be 10")
}

func TestStats_AfterMarkingImages(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	// 标记前3张为 KEEP
	for i := 0; i < 3; i++ {
		err := session.MarkImage(session.queue[i].ID(), shared.ImageActionKeep)
		require.NoError(t, err)
	}

	// 标记中间3张为 PENDING
	for i := 3; i < 6; i++ {
		err := session.MarkImage(session.queue[i].ID(), shared.ImageActionPending)
		require.NoError(t, err)
	}

	// 标记后3张为 REJECT
	for i := 6; i < 9; i++ {
		err := session.MarkImage(session.queue[i].ID(), shared.ImageActionReject)
		require.NoError(t, err)
	}

	stats := session.Stats()

	assert.Equal(t, 10, stats.Total(), "Total should be 10")
	assert.Equal(t, 9, session.CurrentIndex(), "Processed should be 9")
	assert.Equal(t, 3, stats.Kept(), "Kept should be 3")
	assert.Equal(t, 3, stats.Reviewed(), "Reviewed should be 3")
	assert.Equal(t, 3, stats.Rejected(), "Rejected should be 3")
	assert.Equal(t, 1, stats.Remaining(), "Remaining should be 1")
}

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
	// 标记后 CurrentIndex 变为 1，所以应该检查 queue[1] 的 ID
	assert.Equal(t, session.queue[1].ID(), currentImage.ID(), "CurrentImage ID should match second image")
}

func TestMarkImage_ShouldUpdateAction(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	imageID := session.queue[0].ID()
	err := session.MarkImage(imageID, shared.ImageActionKeep)
	require.NoError(t, err)

	assert.Equal(t, 1, session.CurrentIndex(), "CurrentIndex should be 1")
	assert.Equal(t, shared.ImageActionKeep, session.Action(session.queue[0].ID()), "Action should be KEEP")
	assert.True(t, session.CanUndo(), "CanUndo should be true")
}

func TestMarkImage_NonCurrentImage_ShouldFindAndMark(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	imageID := session.queue[2].ID()
	err := session.MarkImage(imageID, shared.ImageActionPending)
	require.NoError(t, err)

	assert.Equal(t, 3, session.CurrentIndex(), "CurrentIndex should be 3")
	assert.Equal(t, shared.ImageActionPending, session.Action(session.queue[2].ID()), "Action should be PENDING")
}

func TestMarkImage_InvalidImageID_ShouldReturnError(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	err := session.MarkImage(scalar.ToID("invalid-id"), shared.ImageActionKeep)
	assert.Error(t, err, "Should return error for invalid image ID")
	assert.Equal(t, ErrSessionNotFound, err, "Error should be ErrSessionNotFound")
}

func TestMarkImage_AllImagesRejected_ShouldCompleteSession(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	markImagesInSession(t, session, func(index int) shared.ImageAction {
		return shared.ImageActionReject
	})

	assert.True(t, session.Stats().IsCompleted(), "IsCompleted should be true")
}

func TestMarkImage_KeepAndReview_ShouldStartNextRound(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	markImagesInSession(t, session, func(index int) shared.ImageAction {
		action := shared.ImageActionKeep
		if index%3 == 0 {
			action = shared.ImageActionPending
		} else if index%3 == 1 {
			action = shared.ImageActionReject
		}
		return action
	})

	assert.False(t, session.Stats().IsCompleted(), "Session should not be completed")

	newRoundStats := session.Stats()
	assert.Equal(t, 0, session.CurrentIndex(), "New round processed should be 0")
	assert.Equal(t, 7, newRoundStats.Total(), "New round total should be 7")
	assert.Equal(t, 7, newRoundStats.Remaining(), "New round remaining should be 7")

	expectedQueueLength := 7
	assert.Equal(t, expectedQueueLength, len(session.queue), "Queue length should be %d", expectedQueueLength)
	assert.Equal(t, 0, session.CurrentIndex(), "CurrentIdx should be 0")
}

func TestMarkImage_KeepAndReview_ShouldStartNextRoundWithBoth(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	markImagesInSession(t, session, func(index int) shared.ImageAction {
		action := shared.ImageActionKeep
		if index%2 == 0 {
			action = shared.ImageActionPending
		}
		return action
	})

	assert.False(t, session.Stats().IsCompleted(), "Session should not be completed")

	expectedQueueLength := 10
	assert.Equal(t, expectedQueueLength, len(session.queue), "Queue length should be %d", expectedQueueLength)
	assert.Equal(t, 0, session.CurrentIndex(), "CurrentIdx should be 0")
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

func TestCanCommit_FirstRoundSingleReject_ShouldBeAbleToCommit(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	err := session.MarkImage(session.queue[0].ID(), shared.ImageActionReject)
	require.NoError(t, err)

	assert.False(t, session.Stats().IsCompleted(), "Session should not be completed")
	assert.True(t, session.CanCommit(), "CanCommit should return true after rejecting one image in first round")

	stats := session.Stats()
	assert.Equal(t, 1, stats.Rejected(), "Expected 1 rejected image")
}

func TestMarkImage_KeptInFirstRound_ShouldKeepStatusInSecondRound(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	keptImageIDs := make(map[scalar.ID]bool)

	markImagesInSession(t, session, func(index int) shared.ImageAction {
		action := shared.ImageActionKeep
		if index%3 == 0 {
			action = shared.ImageActionPending
		} else if index%3 == 1 {
			action = shared.ImageActionReject
		}
		imageID := session.queue[index].ID()
		if action == shared.ImageActionKeep {
			keptImageIDs[imageID] = true
		}
		return action
	})

	assert.False(t, session.Stats().IsCompleted(), "Session should not be completed")

	expectedQueueLength := 7
	assert.Equal(t, expectedQueueLength, len(session.queue), "Queue length should be %d", expectedQueueLength)
	assert.Equal(t, 0, session.CurrentIndex(), "CurrentIdx should be 0")

	for _, img := range session.queue {
		if keptImageIDs[img.ID()] {
			assert.Equal(t, shared.ImageActionKeep, session.Action(img.ID()), "Image %s was marked as KEEP in first round, but action is %s in second round", img.ID(), session.Action(img.ID()))
		}
	}
}

func TestUndo_ShouldRestorePreviousAction(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	imageID := session.queue[0].ID()
	err := session.MarkImage(imageID, shared.ImageActionKeep)
	require.NoError(t, err)

	assert.Equal(t, 1, session.CurrentIndex(), "CurrentIndex should be 1")

	err = session.Undo()
	require.NoError(t, err)

	assert.Equal(t, 0, session.CurrentIndex(), "CurrentIndex should be 0")
	assert.Equal(t, shared.ImageActionPending, session.Action(session.queue[0].ID()), "Action should be restored to PENDING")
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
	session := setupTestSession(t, 10, 5)

	// 完成第一轮筛选，进入第二轮
	markImagesInSession(t, session, func(index int) shared.ImageAction {
		action := shared.ImageActionKeep
		if index%3 == 0 {
			action = shared.ImageActionPending
		} else if index%3 == 1 {
			action = shared.ImageActionReject
		}
		return action
	})

	// 验证会话状态为未完成（第二轮开始）
	assert.False(t, session.Stats().IsCompleted(), "Session should not be completed for second round")

	// 验证 CanUndo 返回 true（支持跨轮撤销）
	assert.True(t, session.CanUndo(), "CanUndo should return true after round completion for cross-round undo")

	// 执行跨轮撤销
	err := session.Undo()
	require.NoError(t, err)

	// 验证撤销成功，回到第一轮状态
	assert.False(t, session.Stats().IsCompleted(), "Session should not be completed after cross-round undo")
	assert.Equal(t, 10, len(session.queue), "Queue length should be restored to original 10")
	assert.Equal(t, 9, session.CurrentIndex(), "CurrentIndex should be restored to last processed index (9) after cross-round undo")
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
		kept:      2,
		reviewed:  2,
		rejected:  1,
		remaining: 5,
	}

	assert.Equal(t, 10, stats.Total(), "Total should match")
	assert.Equal(t, 2, stats.Kept(), "Kept should match")
	assert.Equal(t, 2, stats.Reviewed(), "Reviewed should match")
	assert.Equal(t, 1, stats.Rejected(), "Rejected should match")
	assert.Equal(t, 5, stats.Remaining(), "Remaining should match")
}

func TestSessionError_Error(t *testing.T) {
	err := &SessionError{message: "test error"}

	assert.Equal(t, "test error", err.Error(), "Error message should match")
}

func createTestImages(count int) []*image.Image {
	images := make([]*image.Image, count)
	for i := 0; i < count; i++ {
		images[i] = image.NewImage(
			scalar.ToID(fmt.Sprintf("img-%d", i)),
			"test.jpg",
			fmt.Sprintf("/test/test-%d.jpg", i),
			1000,
			time.Now(),
			nil,
			1920,
			1080,
		)
	}
	return images
}

func createTestImagesWithRatings(ratings []int) []*image.Image {
	images := make([]*image.Image, len(ratings))
	for i, rating := range ratings {
		xmpData := metadata.NewXMPData(rating, "", time.Time{})
		images[i] = image.NewImage(
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

// setupTestSession 创建并返回一个测试用的会话对象
func setupTestSession(t *testing.T, imageCount int, targetKeep int) *Session {
	filter := &shared.ImageFilters{Rating: []int{0}}
	images := createTestImages(imageCount)
	session := NewSession(scalar.ToID("test-id"), scalar.ToID("test-dir-id"), filter, targetKeep, images)
	return session
}

// markImagesInSession 批量标记会话中的图片
func markImagesInSession(t *testing.T, session *Session, actionFn func(index int) shared.ImageAction) {
	for i := 0; i < len(session.queue); i++ {
		action := actionFn(i)
		err := session.MarkImage(session.queue[i].ID(), action)
		require.NoError(t, err)
	}
}

func TestUndo_ShouldRestoreToPreviousRound(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	markImagesInSession(t, session, func(index int) shared.ImageAction {
		action := shared.ImageActionKeep
		if index%3 == 0 {
			action = shared.ImageActionPending
		} else if index%3 == 1 {
			action = shared.ImageActionReject
		}
		return action
	})

	assert.False(t, session.Stats().IsCompleted(), "Session should not be completed after first round")
	assert.Equal(t, 7, len(session.queue), "Queue should have 7 images for second round")
	assert.Equal(t, 0, session.CurrentIndex(), "CurrentIndex should be 0 for second round")

	err := session.MarkImage(session.queue[0].ID(), shared.ImageActionKeep)
	require.NoError(t, err)

	err = session.Undo()
	require.NoError(t, err)

	assert.Equal(t, shared.ImageActionPending, session.Action(session.queue[0].ID()), "Action should be restored to PENDING after undo in second round")
	assert.Equal(t, 0, session.CurrentIndex(), "CurrentIndex should be 0 after undo")
}

func TestUndo_ShouldRestoreToPreviousRoundWhenUndoStackEmpty(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	markImagesInSession(t, session, func(index int) shared.ImageAction {
		action := shared.ImageActionKeep
		if index%3 == 0 {
			action = shared.ImageActionPending
		} else if index%3 == 1 {
			action = shared.ImageActionReject
		}
		return action
	})

	assert.False(t, session.Stats().IsCompleted(), "Session should not be completed after first round")
	assert.Equal(t, 7, len(session.queue), "Queue should have 7 images for second round")
	assert.Equal(t, 1, session.currentRound, "CurrentRound should be 1")

	err := session.Undo()
	require.NoError(t, err)

	assert.Equal(t, 0, session.currentRound, "CurrentRound should be 0 after undo to previous round")
	assert.Equal(t, 10, len(session.queue), "Queue should be restored to 10 images")
	assert.Equal(t, 9, session.CurrentIndex(), "CurrentIndex should be 9 after undo to previous round")
	assert.False(t, session.Stats().IsCompleted(), "Session should not be completed after undo")
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

func TestUndo_ShouldHandleNoMoreUndoActions(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	markImagesInSession(t, session, func(index int) shared.ImageAction {
		return shared.ImageActionReject
	})

	assert.True(t, session.Stats().IsCompleted(), "Session should be completed")

	err := session.Undo()
	require.NoError(t, err)

	assert.False(t, session.Stats().IsCompleted(), "Session should not be completed after undo")
	assert.Equal(t, shared.ImageActionPending, session.Action(session.queue[9].ID()), "Last image action should be restored to PENDING")
}

func TestUndo_CrossRoundToBeginning(t *testing.T) {
	// 创建一个只有 2 张图片的会话，目标保留 1 张
	session := setupTestSession(t, 2, 1)

	img0ID := session.queue[0].ID()
	img1ID := session.queue[1].ID()

	// 1. 标记第一张 (idx 0 -> 1)
	err := session.MarkImage(img0ID, shared.ImageActionKeep)
	require.NoError(t, err)

	// 2. 标记第二张 (idx 1 -> 2) -> 触发下一轮 (idx 0)
	err = session.MarkImage(img1ID, shared.ImageActionKeep)
	require.NoError(t, err)

	require.Equal(t, 1, session.currentRound)

	// 3. 执行第一次撤销：回到第一轮且当前选中最后一张 (img1)
	err = session.Undo()
	require.NoError(t, err)
	assert.Equal(t, 0, session.currentRound)
	assert.Equal(t, 1, session.currentIdx)
	assert.Equal(t, shared.ImageActionPending, session.Action(img1ID))

	// 4. 执行第二次撤销：撤销 img0 的标记 (idx 1 -> 0)
	err = session.Undo()
	require.NoError(t, err)
	assert.Equal(t, 0, session.currentIdx)
	assert.Equal(t, shared.ImageActionPending, session.Action(img0ID))

	// 5. 执行第三次撤销：预期返回错误，且不会 panic
	err = session.Undo()
	assert.Equal(t, ErrNothingToUndo, err)
	assert.Equal(t, 0, session.currentIdx)

	// 验证之后依然可以正常操作
	err = session.MarkImage(img0ID, shared.ImageActionKeep)
	require.NoError(t, err)
	assert.Equal(t, 1, session.currentIdx)
}

func TestUndo_ShouldRestoreIndex_WhenRemarking(t *testing.T) {
	// 验证当对同一张图片进行多次操作后撤销，索引能够正确恢复
	// 用于防止 "index out of range" panic (当盲目递减 currentIdx 时可能发生)
	session := setupTestSession(t, 2, 1)
	img0ID := session.queue[0].ID()

	// 1. 标记第一张 idx: 0 -> 1
	err := session.MarkImage(img0ID, shared.ImageActionKeep)
	require.NoError(t, err)
	assert.Equal(t, 1, session.currentIdx)

	// 2. 再次标记同一张图片 (模拟用户回退或者跳转后重新标记)
	// 在 MarkImage 内部，如果发现 ID 不同，会重置 currentIdx。但这里 ID 相同 (虽然在实际逻辑中 MarkImage 主要是处理当前 currentIdx 指向的图片，
	// 或者通过便利查找图片。如果 currentIdx 已经指向下一张了，再次传入 img0ID，MarkImage 会先查找这一张图片，把 currentIdx 设为 0，然后处理完 ++ 变为 1)
	err = session.MarkImage(img0ID, shared.ImageActionKeep)
	require.NoError(t, err)
	assert.Equal(t, 1, session.currentIdx)
	assert.Equal(t, 2, len(session.undoStack))

	// 3. 撤销第二次标记
	// 应该恢复到第二次标记前的状态 (currentIdx 应为 0，因为第二次标记前它被找到了并设为了 0)
	// 注意：MarkImage 逻辑：
	//    currentIdx = 1 (指向 img1)
	//    传入 img0ID
	//    检查 queue[1].ID != img0ID
	//    查找 img0ID -> found at index 0. currentIdx = 0.
	//    record undo (index=0)
	//    Mark...
	//    currentIdx++ -> 1
	// 所以第一次撤销应该恢复 index=0
	err = session.Undo()
	require.NoError(t, err)
	assert.Equal(t, 0, session.currentIdx)
	assert.Equal(t, 1, len(session.undoStack))

	// 4. 撤销第一次标记
	// 恢复到 index=0
	err = session.Undo()
	require.NoError(t, err)
	assert.Equal(t, 0, session.currentIdx)

	// 验证没有变成负数
	assert.True(t, session.currentIdx >= 0, "currentIdx should not be negative")

	// 验证可以继续操作
	err = session.MarkImage(img0ID, shared.ImageActionKeep)
	require.NoError(t, err)
}
