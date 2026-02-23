package session

import (
	"main/internal/domain/image"
	"main/internal/domain/metadata"
	"main/internal/scalar"
	"main/internal/shared"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoveImageByPath_ShouldRemoveUnmarkedImage(t *testing.T) {
	session := setupTestSession(t, 3, 5)

	img0 := session.images[session.queue[0]]
	originalSize := len(session.queue)

	removed := session.RemoveImageByPath(img0.Path())

	assert.True(t, removed, "未操作的图片应该被移除")
	assert.Equal(t, originalSize-1, len(session.queue), "队列长度应减少 1")
}

func TestRemoveImageByPath_ShouldNotRemoveImageWithAction(t *testing.T) {
	session := setupTestSession(t, 3, 5)

	img0 := session.images[session.queue[0]]
	require.NoError(t, session.MarkImage(img0.ID(), shared.ImageActionKeep))

	originalSize := len(session.queue)
	removed := session.RemoveImageByPath(img0.Path())

	assert.False(t, removed, "已操作的图片不应该被移除")
	assert.Equal(t, originalSize, len(session.queue), "队列长度不应改变")
}

func TestRemoveImageByPath_ShouldNotRemoveImageWithRejectAction(t *testing.T) {
	session := setupTestSession(t, 3, 5)

	img0 := session.images[session.queue[0]]
	require.NoError(t, session.MarkImage(img0.ID(), shared.ImageActionReject))

	removed := session.RemoveImageByPath(img0.Path())

	assert.False(t, removed, "已 Reject 的图片同样不应该被移除")
}

// TestUpdateImage_ShouldNotRemoveMarkedImageWhenFilterChanges 模拟 Commit 后文件 rating 变化场景：
// 筛选条件是 rating=0，Commit 写入 rating=5，文件监听器收到变更，
// 图片不再符合过滤器 → 但因为已有 action，不应从队列移除
func TestUpdateImage_ShouldNotRemoveMarkedImageWhenFilterChanges(t *testing.T) {
	// filter: rating=0
	filter := &shared.ImageFilters{Rating: []int{0}}
	xmpRating0 := metadata.NewXMPData(0, "", time.Time{})
	img := image.NewImage(
		scalar.ToID("img-0"),
		"test.jpg",
		"/test/test-0.jpg",
		1000,
		time.Now(),
		xmpRating0,
		1920,
		1080,
	)

	session := NewSession(scalar.ToID("s1"), scalar.ToID("d1"), filter, 5, []*image.Image{img})

	// 用户操作：标记为 Keep
	require.NoError(t, session.MarkImage(img.ID(), shared.ImageActionKeep))
	assert.Equal(t, 1, len(session.queue))

	// 模拟 Commit 后文件 rating 变为 5，文件监听器触发 UpdateImage
	xmpRating5 := metadata.NewXMPData(5, "Keep", time.Now())
	updatedImg := image.NewImage(
		img.ID(), // 同一个 ID（ModTime 未变）
		img.Filename(),
		img.Path(),
		img.Size(),
		img.ModTime(),
		xmpRating5,
		img.Width(),
		img.Height(),
	)

	// rating=5 不符合 filter(rating=0)，matchesFilter=false
	filterFunc := image.BuildImageFilter(filter)
	changed := session.UpdateImage(updatedImg, filterFunc(updatedImg))

	// 因为图片已有 action，不应被移除
	assert.False(t, changed, "已操作图片不应被移除，changed 应为 false")
	assert.Equal(t, 1, len(session.queue), "队列长度不应改变，图片仍应在队列中")
	assert.Equal(t, shared.ImageActionKeep, ActionOf(session, img.ID()), "action 记录不应丢失")

	// 图片已被标记过（MarkImage 导致 currentIdx=1），所以 Remaining=0，IsCompleted=true
	// 重点：如果没有修复（图片被错误移除），stats.Kept 会变为 0，session 依然 IsCompleted，
	// 但后续无法 undo 也无法知道已标记的图片去哪了
	stats := session.Stats()
	assert.Equal(t, 0, stats.Remaining, "图片已处理完，Remaining 应为 0")
	assert.Equal(t, 1, stats.Kept, "Kept 计数应保留（图片仍在 images 中）")
}
