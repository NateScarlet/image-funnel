package session

import (
	"main/internal/domain/image"
	"main/internal/domain/metadata"
	"main/internal/scalar"
	"main/internal/shared"
	"main/internal/util"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoveImageByPath_ShouldRemoveUnmarkedImage(t *testing.T) {
	session := setupTestSession(t, 3, 5)

	img0 := session.images[session.queue[0]]
	originalSize := len(session.queue)

	removed, err := session.removeImageByRelPath(img0.RelPath(), false)
	require.NoError(t, err)

	assert.True(t, removed, "未操作的图片应该被移除")
	assert.Equal(t, originalSize-1, len(session.queue), "队列长度应减少 1")
}

func TestRemoveImageByPath_ShouldNotRemoveImageWithAction(t *testing.T) {
	session := setupTestSession(t, 3, 5)

	img0 := session.images[session.queue[0]]
	require.NoError(t, session.MarkImage(img0.ID(), shared.ImageActionKeep))

	originalSize := len(session.queue)
	removed, err := session.removeImageByRelPath(img0.RelPath(), false)
	require.NoError(t, err)

	assert.False(t, removed, "已操作的图片不应该被移除")
	assert.Equal(t, originalSize, len(session.queue), "队列长度不应改变")
}

func TestRemoveImageByPath_ShouldNotRemoveImageWithRejectAction(t *testing.T) {
	session := setupTestSession(t, 3, 5)

	img0 := session.images[session.queue[0]]
	require.NoError(t, session.MarkImage(img0.ID(), shared.ImageActionReject))

	removed, err := session.removeImageByRelPath(img0.RelPath(), false)
	require.NoError(t, err)

	assert.False(t, removed, "已 Reject 的图片同样不应该被移除")
}

// TestUpdateImage_ShouldNotRemoveMarkedImageWhenFilterChanges 模拟 Commit 后文件 rating 变化场景：
// 筛选条件是 rating=0，Commit 写入 rating=5，文件监听器收到变更，
// 图片不再符合过滤器 → 但因为已有 action，不应从队列移除
func TestUpdateImage_ShouldNotRemoveMarkedImageWhenFilterChanges(t *testing.T) {
	// filter: rating=0
	filter := &shared.ImageFilters{Rating: []int{0}}
	xmpRating0 := metadata.NewXMPData(0, "", time.Time{}, "")
	img := image.New(
		scalar.ToID("img-0"),
		"test.jpg",
		"test-0.jpg",
		scalar.ToID("d1"),
		1000,
		time.Now(),
		xmpRating0,
		1920,
		1080,
	)

	session := New(scalar.ToID("s1"), scalar.ToID("d1"), filter, 5, []*image.Image{img}, image.NewFilterBuilder())

	// 用户操作：标记为 Keep
	require.NoError(t, session.MarkImage(img.ID(), shared.ImageActionKeep))
	assert.Equal(t, 1, len(session.queue))

	// 模拟 Commit 后文件 rating 变为 5，文件监听器触发 UpdateImage
	xmpRating5 := metadata.NewXMPData(5, "Keep", time.Now(), "")
	updatedImg := image.New(
		img.ID(), // 同一个 ID（ModTime 未变）
		img.Filename(),
		img.RelPath(),
		img.DirectoryID(),
		img.Size(),
		img.ModTime(),
		xmpRating5,
		img.Width(),
		img.Height(),
	)

	// rating=5 不符合 filter(rating=0)，matchesFilter=false
	filterFunc := session.imageFilterBuilder.Build(util.UnwrapPointer(filter))
	changed, err := session.UpdateImage(updatedImg, filterFunc(updatedImg))
	require.NoError(t, err)

	// 因为图片已有 action，不应被移除
	assert.False(t, changed, "已操作图片不应被移除，changed 应为 false")
	assert.Equal(t, 1, len(session.queue), "队列长度不应改变，图片仍应在队列中")
	assert.Equal(t, shared.ImageActionKeep, ActionOf(session, img.ID()), "action 记录不应丢失")

	// 图片已被标记过（MarkImage 导致 currentIdx=1），所以 Remaining=0，IsCompleted=true
	// 重点：如果没有修复（图片被错误移除），stats.TotalKept 会变为 0，session 依然 IsCompleted，
	// 但后续无法 undo 也无法知道已标记的图片去哪了
	stats := session.Stats()
	assert.Equal(t, 0, stats.CurrentRoundRemaining, "图片已处理完，Remaining 应为 0")
	assert.Equal(t, 1, stats.TotalKept, "Kept 计数应保留（图片仍在 images 中）")
}

func TestRemoveImageByPath_ShouldRemoveImageWithActionIfForce(t *testing.T) {
	session := setupTestSession(t, 3, 5)

	img0 := session.images[session.queue[0]]
	require.NoError(t, session.MarkImage(img0.ID(), shared.ImageActionKeep))

	originalSize := len(session.queue)
	removed, err := session.removeImageByRelPath(img0.RelPath(), true)
	require.NoError(t, err)

	assert.True(t, removed, "即便已操作，强制移除也应该成功")
	assert.Equal(t, originalSize-1, len(session.queue), "队列长度应减少 1")
}

func TestRemoveImageByPath_ForceRemoveThenUndo_ShouldNotRestoreIndex(t *testing.T) {
	session := setupTestSession(t, 3, 5)

	img0 := session.images[session.queue[0]]
	require.NoError(t, session.MarkImage(img0.ID(), shared.ImageActionKeep))

	// 此时 img0 已经被标记，currentIdx 为 1
	assert.Equal(t, 1, session.currentIdx)

	// 强制删除 img0
	removed, err := session.removeImageByRelPath(img0.RelPath(), true)
	require.NoError(t, err)
	assert.True(t, removed)

	// 队列长度变短为 2，由于 targetIndex (0) < currentIdx (1)，currentIdx 应该自动递减为 0
	assert.Equal(t, 0, session.currentIdx)
	assert.Equal(t, 2, len(session.queue))

	// 此时执行 Undo 撤销标记 img0
	err = session.Undo()
	assert.NoError(t, err)

	// action 应该被撤销
	assert.Empty(t, session.actions[img0.ID()])

	// 即使撤销了已删除图片的标记，currentIdx 也应保持安全位置而不受影响
	assert.Equal(t, 0, session.currentIdx)
}

func TestRemoveImageByPath_ForceRemoveThenUndo_Complex(t *testing.T) {
	session := setupTestSession(t, 3, 5) // queue: [img0, img1, img2]

	img0 := session.images[session.queue[0]]
	img1 := session.images[session.queue[1]]

	require.NoError(t, session.MarkImage(img0.ID(), shared.ImageActionKeep))   // currentIdx -> 1
	require.NoError(t, session.MarkImage(img1.ID(), shared.ImageActionReject)) // currentIdx -> 2

	// 此时队列有 3 个，当前指向 img2 (idx=2)
	assert.Equal(t, 2, session.currentIdx)

	// 强制删除 img1 (idx=1)
	removed, err := session.removeImageByRelPath(img1.RelPath(), true)
	require.NoError(t, err)
	assert.True(t, removed)

	// 移除了 idx=1 的图。由于 targetIndex (1) < currentIdx (2)，currentIdx 变为 1。
	// 队列剩下 [img0, img2]，currentIdx 指向原 img2 (新 idx=1)。
	assert.Equal(t, 1, session.currentIdx)

	// 撤销对 img1 的操作 (Reject)
	err = session.Undo()
	assert.NoError(t, err)

	// img1 的 action 被清除
	assert.Empty(t, session.actions[img1.ID()])

	// 因为 img1 不在队列中，撤销时跳过对其 currentIdx 的还原，currentIdx 应当保持指向 img2 (idx=1)
	assert.Equal(t, 1, session.currentIdx)
}

func TestRemoveImageByPath_ForceRemoveShouldUpdateStats(t *testing.T) {
	session := setupTestSession(t, 3, 2) // queue: [img0, img1, img2], targetKeep: 2

	img0 := session.images[session.queue[0]]

	// 1. 用户将 img0 标记为 Keep
	require.NoError(t, session.MarkImage(img0.ID(), shared.ImageActionKeep))

	// 检查统计，TotalKept 应为 1
	assert.Equal(t, 1, session.Stats().TotalKept)

	// 2. 强制移除了已被标记为 Keep 的 img0
	removed, err := session.removeImageByRelPath(img0.RelPath(), true)
	require.NoError(t, err)
	assert.True(t, removed)

	// 3. 验证：强制移除后，被移除的 img0 应当不被计入 TotalKept
	assert.Equal(t, 0, session.Stats().TotalKept, "被删除图片的 Keep 操作不应影响统计的 TotalKept")
}

