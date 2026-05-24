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

func TestSession_MarkedButNotWritten_AfterNextRound(t *testing.T) {
	imgA := image.New(
		scalar.ToID("img-a"),
		"test.jpg",
		"/test/test-a.jpg",
		scalar.ToID("d1"),
		1000,
		time.Now(),
		nil,
		1920,
		1080,
	)

	filter := &shared.ImageFilters{}
	session := New(scalar.ToID("s1"), scalar.ToID("d1"), filter, 10, []*image.Image{imgA})

	assert.Equal(t, 1, len(ImagesOf(session)))

	session.RemoveImageByAbsPath(imgA.AbsPath())
	assert.Equal(t, 0, session.CurrentSize())

	imgAFresh := image.New(
		scalar.ToID("img-a"),
		"test.jpg",
		"/test/test-a.jpg",
		scalar.ToID("d1"),
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

func TestSession_NextRound_ShouldAvoidContinuousImage(t *testing.T) {
	// 场景一：正常完成当前轮（即 prevIdx 刚好增加到队列长度，表示当前轮所有图片已全部标记完毕）
	t.Run("complete round", func(t *testing.T) {
		images := createTestImages(3) // img-0, img-1, img-2
		filter := &shared.ImageFilters{}
		// 设置目标保留数为 1。当 3 张图都被标记为 Keep 时，会因为 Keep 数量大于目标数而自动触发换轮
		session := New(scalar.ToID("s1"), scalar.ToID("d1"), filter, 1, images)

		// 显式指定不同的操作耗时。
		// 由于换轮时新队列会根据耗时升序排序，我们将最后一张 img-2 耗时设为最短（1ms），而其余设为 10ms，
		// 这样在不施加防连续约束的情况下，下一轮的第一张必然是 img-2，从而造成连续评价同一张图的不好体验。
		err := session.MarkImage(scalar.ToID("img-0"), shared.ImageActionKeep, shared.WithDuration(scalar.DurationFromStandard(10*time.Millisecond)))
		require.NoError(t, err)
		err = session.MarkImage(scalar.ToID("img-1"), shared.ImageActionKeep, shared.WithDuration(scalar.DurationFromStandard(10*time.Millisecond)))
		require.NoError(t, err)

		// 标记最后一张图片触发自动换轮
		err = session.MarkImage(scalar.ToID("img-2"), shared.ImageActionKeep, shared.WithDuration(scalar.DurationFromStandard(1*time.Millisecond)))
		require.NoError(t, err)

		// 验证是否已成功换轮
		assert.Equal(t, 1, session.CurrentRound())

		// 原排序为 img-2, img-0, img-1。
		// 因 img-2 是上一轮 the last image，防连续约束应生效，对调前两位，使得最终队列为 img-0, img-2, img-1。
		require.Equal(t, 3, session.CurrentSize())
		assert.Equal(t, scalar.ToID("img-0"), session.images[session.queue[0]].ID())
		assert.Equal(t, scalar.ToID("img-2"), session.images[session.queue[1]].ID())
		assert.Equal(t, scalar.ToID("img-1"), session.images[session.queue[2]].ID())
	})

	// 场景二：中途通过外部干预直接换轮（此时当前轮未完成，prevIdx 依然小于当前队列长度）
	t.Run("incomplete round", func(t *testing.T) {
		images := createTestImages(3) // img-0, img-1, img-2
		filter := &shared.ImageFilters{}
		session := New(scalar.ToID("s1"), scalar.ToID("d1"), filter, 1, images)

		// 仅标记首张图片，此时 currentIdx 为 1，即指向队列中的第二张图片 img-1（用户当前正在浏览/处理的图片）
		err := session.MarkImage(scalar.ToID("img-0"), shared.ImageActionKeep, shared.WithDuration(scalar.DurationFromStandard(1*time.Millisecond)))
		require.NoError(t, err)

		// 模拟中途进行过滤条件变更或手动切换等导致的主动换轮。
		// 传入包含 img-1 和 img-2 的新队列。此时因为 img-1 耗时最短（0ms），正常排序后第一张是 img-1。
		// 由于 img-1 是正在看的图片，如果继续作为新一轮的第一张会让用户觉得没有切换，防连续约束应当对其做对调处理。
		err = session.NextRound(filter, []*image.Image{images[1], images[2]})
		require.NoError(t, err)

		assert.Equal(t, 1, session.CurrentRound())

		// 原排序为 img-1, img-2。对调后应为 img-2, img-1。
		require.Equal(t, 2, session.CurrentSize())
		assert.Equal(t, scalar.ToID("img-2"), session.images[session.queue[0]].ID())
		assert.Equal(t, scalar.ToID("img-1"), session.images[session.queue[1]].ID())
	})

	// 场景三：无冲突情况（下一轮排序后的首张图片本来就和上一轮的最后一张不同）
	t.Run("no exchange needed", func(t *testing.T) {
		images := createTestImages(3) // img-0, img-1, img-2
		filter := &shared.ImageFilters{}
		session := New(scalar.ToID("s1"), scalar.ToID("d1"), filter, 1, images)

		// 将 img-0 设为最短耗时（1ms），而最后一张 img-2 设为 10ms
		err := session.MarkImage(scalar.ToID("img-0"), shared.ImageActionKeep, shared.WithDuration(scalar.DurationFromStandard(1*time.Millisecond)))
		require.NoError(t, err)
		err = session.MarkImage(scalar.ToID("img-1"), shared.ImageActionKeep, shared.WithDuration(scalar.DurationFromStandard(10*time.Millisecond)))
		require.NoError(t, err)
		err = session.MarkImage(scalar.ToID("img-2"), shared.ImageActionKeep, shared.WithDuration(scalar.DurationFromStandard(10*time.Millisecond)))
		require.NoError(t, err)

		// 上一轮最后一张是 img-2。按照耗时排序，新一轮队列为 img-0 (1ms), img-1 (10ms), img-2 (10ms)。
		// 第一张为 img-0，不存在冲突，所以应保持原样，不进行任何对调。
		require.Equal(t, 3, session.CurrentSize())
		assert.Equal(t, scalar.ToID("img-0"), session.images[session.queue[0]].ID())
		assert.Equal(t, scalar.ToID("img-1"), session.images[session.queue[1]].ID())
		assert.Equal(t, scalar.ToID("img-2"), session.images[session.queue[2]].ID())
	})

	// 场景四：新一轮队列只有单张图片（即使这唯一的一张图与上一轮最后一张相同，也无法且无需对调）
	t.Run("single image no exchange", func(t *testing.T) {
		images := createTestImages(3) // img-0, img-1, img-2
		filter := &shared.ImageFilters{}
		// 将目标保留数量设为 0。当有图片被 Keep 时（哪怕只有 1 张），由于 1 > 0，也会触发换轮
		session := New(scalar.ToID("s1"), scalar.ToID("d1"), filter, 0, images)

		err := session.MarkImage(scalar.ToID("img-0"), shared.ImageActionReject)
		require.NoError(t, err)
		err = session.MarkImage(scalar.ToID("img-1"), shared.ImageActionReject)
		require.NoError(t, err)

		// 仅保留最后一张 img-2。上一轮最后一张是 img-2，新队列中也只有 img-2。
		err = session.MarkImage(scalar.ToID("img-2"), shared.ImageActionKeep)
		require.NoError(t, err)

		// 验证只有一张图片被保留，且没有报错，队列保持原样
		require.Equal(t, 1, session.CurrentSize())
		assert.Equal(t, scalar.ToID("img-2"), session.images[session.queue[0]].ID())
	})
}
