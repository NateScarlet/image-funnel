package session

import (
	"main/internal/apperror"
	"main/internal/domain/image"
	"main/internal/scalar"
	"main/internal/shared"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarkImage_ShouldUpdateAction(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	imageID := session.images[session.queue[0]].ID()
	err := session.MarkImage(imageID, shared.ImageActionKeep)
	require.NoError(t, err)

	assert.Equal(t, 1, session.CurrentIndex(), "CurrentIndex should be 1")
	assert.Equal(t, shared.ImageActionKeep, ActionOf(session, session.images[session.queue[0]].ID()), "Action should be KEEP")
	assert.True(t, session.CanUndo(), "CanUndo should be true")
}

// 乱序标记（非当前图片）应直接覆盖操作记录，不报错，不移动队列位置
func TestMarkImage_OutOfOrderImage_ShouldOverwriteAction(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	// 标记队列中第 2 个（未到达的）图片
	imageID := session.images[session.queue[2]].ID()
	err := session.MarkImage(imageID, shared.ImageActionShelve)
	assert.NoError(t, err, "乱序标记队列内图片不应报错")
	assert.Equal(t, 0, session.CurrentIndex(), "乱序标记不应推进队列位置")
	assert.Equal(t, shared.ImageActionShelve, ActionOf(session, imageID), "操作记录应被覆盖")

	_ = apperror.ErrCode // 保持导入被使用（InvalidImageID 测试仍需要它）
}

func TestMarkImage_InvalidImageID_ShouldReturnError(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	err := session.MarkImage(scalar.ToID("invalid-id"), shared.ImageActionKeep)
	assert.Error(t, err, "Should return error for invalid image ID")
	assert.True(t, apperror.IsNotFound(err), "Error should be not found error")
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

func TestSession_MarkImage_Sorting(t *testing.T) {
	img1 := image.NewImage(scalar.ToID("img1"), "img1.jpg", "/path/to/img1.jpg", 1000, time.Now(), nil, 100, 100)
	img2 := image.NewImage(scalar.ToID("img2"), "img2.jpg", "/path/to/img2.jpg", 1000, time.Now(), nil, 100, 100)
	img3 := image.NewImage(scalar.ToID("img3"), "img3.jpg", "/path/to/img3.jpg", 1000, time.Now(), nil, 100, 100)
	images := []*image.Image{img1, img2, img3}

	sess := NewSession(scalar.ToID("sessSort"), scalar.ToID("dir1"), nil, 1, images)

	mark := func(id scalar.ID, durationMs int64) {
		opts := []shared.MarkImageOption{}
		if durationMs > 0 {
			d := scalar.NewDuration(scalar.DurationWithMilliseconds(durationMs))
			opts = append(opts, shared.WithDuration(d))
		}
		err := sess.MarkImage(id, shared.ImageActionKeep, opts...)
		require.NoError(t, err)
	}

	mark(scalar.ToID("img1"), 3000)
	mark(scalar.ToID("img2"), 1000)
	mark(scalar.ToID("img3"), 2000)

	assert.Equal(t, 1, sess.currentRound)
	assert.Equal(t, 3, len(sess.queue))

	assert.Equal(t, scalar.ToID("img2"), sess.images[sess.queue[0]].ID())
	assert.Equal(t, scalar.ToID("img3"), sess.images[sess.queue[1]].ID())
	assert.Equal(t, scalar.ToID("img1"), sess.images[sess.queue[2]].ID())
}

func TestSession_MarkImage_DurationAccumulation(t *testing.T) {
	img1 := image.NewImage(scalar.ToID("img1"), "img1.jpg", "/path/to/img1.jpg", 1000, time.Now(), nil, 100, 100)
	images := []*image.Image{img1}
	sess := NewSession(scalar.ToID("sessAcc"), scalar.ToID("dir1"), nil, 10, images)

	// 1. Mark with 1000ms
	d1 := scalar.NewDuration(scalar.DurationWithMilliseconds(1000))
	err := sess.MarkImage(scalar.ToID("img1"), shared.ImageActionKeep, shared.WithDuration(d1))
	require.NoError(t, err)

	assert.Equal(t, 1000.0, sess.durations[scalar.ToID("img1")].Milliseconds())

	// 2. Undo
	err = sess.Undo()
	require.NoError(t, err)
	// Duration should persist
	assert.Equal(t, 1000.0, sess.durations[scalar.ToID("img1")].Milliseconds(), "Duration should persist after undo")

	// 3. Mark again with 2000ms (Total should be 3000ms)
	d2 := scalar.NewDuration(scalar.DurationWithMilliseconds(2000))
	err = sess.MarkImage(scalar.ToID("img1"), shared.ImageActionKeep, shared.WithDuration(d2))
	require.NoError(t, err)
	assert.Equal(t, 3000.0, sess.durations[scalar.ToID("img1")].Milliseconds())
}

func TestSession_MarkImage_AvoidConsecutiveSameImage(t *testing.T) {
	img1 := image.NewImage(scalar.ToID("img1"), "img1.jpg", "/path/to/img1.jpg", 1000, time.Now(), nil, 100, 100)
	img2 := image.NewImage(scalar.ToID("img2"), "img2.jpg", "/path/to/img2.jpg", 1000, time.Now(), nil, 100, 100)
	img3 := image.NewImage(scalar.ToID("img3"), "img3.jpg", "/path/to/img3.jpg", 1000, time.Now(), nil, 100, 100)
	images := []*image.Image{img1, img3, img2}

	sess := NewSession(scalar.ToID("sessAvoid"), scalar.ToID("dir1"), nil, 1, images)

	mark := func(id scalar.ID, durationMs int64) {
		d := scalar.NewDuration(scalar.DurationWithMilliseconds(durationMs))
		err := sess.MarkImage(id, shared.ImageActionKeep, shared.WithDuration(d))
		require.NoError(t, err)
	}

	mark(scalar.ToID("img1"), 3000)
	mark(scalar.ToID("img3"), 2000)
	mark(scalar.ToID("img2"), 1000)

	require.Equal(t, 1, sess.currentRound)
	require.Equal(t, 3, len(sess.queue))

	assert.Equal(t, scalar.ToID("img3"), sess.images[sess.queue[0]].ID(), "First image should be img3 (swapped)")
	assert.Equal(t, scalar.ToID("img2"), sess.images[sess.queue[1]].ID(), "Second image should be img2 (swapped)")
	assert.Equal(t, scalar.ToID("img1"), sess.images[sess.queue[2]].ID(), "Third image should be img1")
}

// 复现 Bug：用户在队列末尾（刚完成状态）对之前的图片做乱序标记，
// 使 Kept > targetKeep，此时必须触发 NextRound，否则 currentImage 永远为 null。
func TestMarkImage_OutOfOrder_AtEndOfQueue_ShouldTriggerNextRound(t *testing.T) {
	img1 := image.NewImage(scalar.ToID("img1"), "img1.jpg", "/path/img1.jpg", 1000, time.Now(), nil, 100, 100)
	img2 := image.NewImage(scalar.ToID("img2"), "img2.jpg", "/path/img2.jpg", 1000, time.Now(), nil, 100, 100)
	images := []*image.Image{img1, img2}

	sess := NewSession(scalar.ToID("sess"), scalar.ToID("dir1"), nil, 1, images)

	// 标记 img1 KEEP（推进到 currentIdx=1）
	require.NoError(t, sess.MarkImage(scalar.ToID("img1"), shared.ImageActionKeep))
	assert.Equal(t, 1, sess.currentIdx)

	// 标记 img2 REJECT（推进到 currentIdx=2，到达末尾）
	// Kept=1, targetKeep=1，不触发 NextRound，会话正常完成
	require.NoError(t, sess.MarkImage(scalar.ToID("img2"), shared.ImageActionReject))
	assert.Equal(t, 2, sess.currentIdx)
	assert.True(t, sess.Stats().IsCompleted, "session should be completed at this point")
	assert.Nil(t, sess.CurrentImage(), "currentImage should be nil when completed")

	// 此时处于"已完成"状态，用户将 img2 从 REJECT 改为 KEEP（乱序标记，因为越界了）
	// 使 Kept=2 > targetKeep=1，应触发 NextRound 开启新一轮
	require.NoError(t, sess.MarkImage(scalar.ToID("img2"), shared.ImageActionKeep))

	// 验证：NextRound 被正确触发，currentImage 不再是 null
	assert.NotNil(t, sess.CurrentImage(), "currentImage should not be nil after out-of-order mark triggers NextRound")
	assert.False(t, sess.Stats().IsCompleted, "session should not be completed after NextRound triggered")
	assert.Equal(t, 0, sess.currentIdx, "currentIdx should be reset to 0 by NextRound")
}
