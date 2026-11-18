package session

import (
	"context"
	"main/internal/domain/directory"
	"main/internal/domain/image"
	"main/internal/domain/metadata"
	"main/internal/domain/relpath"
	"main/internal/scalar"
	"main/internal/shared"
	"main/internal/testutil"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// 「下一轮成员」由唯一原语判定：仅按筛选条件挑出候选集中的成员。
func TestSession_NextRoundMembers_AppliesFilter(t *testing.T) {
	images := createTestImagesWithRatings(t, []int{1, 2, 3})
	filter := &shared.ImageFilters{Rating: []int{2, 3}}
	sess := New(scalar.ToID("s1"), scalar.ToID("d1"), nil, 10, images, image.NewFilterBuilder())

	members := sess.nextRoundMembers(images, filter)
	require.Len(t, members, 2)
	assert.Equal(t, images[1].ID(), members[0].ID())
	assert.Equal(t, images[2].ID(), members[1].ID())

	// 原语只认筛选条件，已提交状态不改变成员的判定
	sess.MarkCommitted(images[0].ID())
	members = sess.nextRoundMembers(images, filter)
	require.Len(t, members, 2)
	assert.Equal(t, images[1].ID(), members[0].ID())
}

// 两条路径共用的原语对同一候选集与筛选条件给出相同成员，且不改动候选集。
func TestSession_NextRoundMembers_SameCandidatesSameMembers(t *testing.T) {
	images := createTestImagesWithRatings(t, []int{1, 2, 3})
	filter := &shared.ImageFilters{Rating: []int{1, 3}}
	sess := New(scalar.ToID("s1"), scalar.ToID("d1"), filter, 10, images, image.NewFilterBuilder())

	candidates := []*image.Image{images[0], images[1], images[2]}
	autoPath := sess.nextRoundMembers(candidates, filter)
	filterPath := sess.nextRoundMembers(candidates, filter)

	assert.Equal(t, idsOf(autoPath), idsOf(filterPath))
	assert.Equal(t, []scalar.ID{images[0].ID(), images[1].ID(), images[2].ID()}, idsOf(candidates),
		"原语不应改动传入的候选集")
}

// 自动推进与改筛选对同一批候选集与筛选条件给出一致的新一轮队列。
func TestNextRound_AutoAdvanceAndFilterChangeAgree(t *testing.T) {
	ctx := context.Background()
	filter := &shared.ImageFilters{Rating: []int{1, 3}}
	autoImages := createTestImagesWithRatings(t, []int{1, 2, 3})
	changedImages := createTestImagesWithRatings(t, []int{1, 2, 3})

	// 自动推进：Keep 两张、排除一张，Keep 数超过目标触发换轮
	autoSess := New(scalar.ToID("auto"), scalar.ToID("d1"), filter, 1, autoImages, image.NewFilterBuilder())
	require.NoError(t, autoSess.MarkImage(autoImages[0].ID(), shared.ImageActionKeep))
	require.NoError(t, autoSess.MarkImage(autoImages[1].ID(), shared.ImageActionReject))
	require.NoError(t, autoSess.MarkImage(autoImages[2].ID(), shared.ImageActionKeep))
	require.Equal(t, 1, autoSess.CurrentRound())

	// 改筛选：相同的标记进度，但目标足够大不触发自动推进，再由 Update 换轮
	changedSess := New(scalar.ToID("changed"), scalar.ToID("d1"), filter, 10, changedImages, image.NewFilterBuilder())
	metaRepo := NewFakeMetadataRepo()
	imgRepo := &FakeImageRepo{
		MetaRepo: metaRepo,
		Images:   map[relpath.Path]*image.Image{},
	}
	for _, img := range changedImages {
		imgRepo.Images[img.RelPath()] = img
	}
	sessionRepo := &FakeSessionRepo{Sessions: map[scalar.ID]*Session{changedSess.ID(): changedSess}}
	svc, cleanup := newNextRoundTestService(t, sessionRepo, metaRepo, imgRepo, &mockSessionSaved{ids: make(chan scalar.ID, 4)})
	defer cleanup()

	require.NoError(t, changedSess.MarkImage(changedImages[0].ID(), shared.ImageActionKeep))
	require.NoError(t, changedSess.MarkImage(changedImages[1].ID(), shared.ImageActionReject))
	require.NoError(t, changedSess.MarkImage(changedImages[2].ID(), shared.ImageActionKeep))
	require.NoError(t, svc.Update(ctx, changedSess.ID(), WithFilter(filter)))
	require.Equal(t, 1, changedSess.CurrentRound())

	assert.Equal(t, queueIDsOf(autoSess), queueIDsOf(changedSess))
	assert.Equal(t, []scalar.ID{autoImages[0].ID(), autoImages[2].ID()}, queueIDsOf(autoSess))
}

func queueIDsOf(sess *Session) []scalar.ID {
	ids := make([]scalar.ID, 0, len(sess.queue))
	for _, idx := range sess.queue {
		ids = append(ids, sess.images[idx].ID())
	}
	return ids
}

// NextRound 内部会按耗时排序并做防连续对调，但不应改动调用者传入的切片。
func TestSession_NextRound_DoesNotMutateCallerSlice(t *testing.T) {
	images := createTestImages(t, 3)
	sess := New(scalar.ToID("s1"), scalar.ToID("d1"), &shared.ImageFilters{}, 10, images, image.NewFilterBuilder())

	// 排序后首张仍是 images[0]，与上一轮当前图片相同 → 触发防连续对调
	sess.durations[images[0].ID()] = scalar.DurationFromStandard(1 * time.Millisecond)
	sess.durations[images[1].ID()] = scalar.DurationFromStandard(5 * time.Millisecond)
	sess.durations[images[2].ID()] = scalar.DurationFromStandard(10 * time.Millisecond)

	candidates := []*image.Image{images[0], images[1], images[2]}
	require.NoError(t, sess.NextRound(nil, candidates))

	assert.Equal(t, []scalar.ID{images[0].ID(), images[1].ID(), images[2].ID()}, idsOf(candidates),
		"调用者切片不应被排序或对调")
	assert.Equal(t, []scalar.ID{images[1].ID(), images[0].ID(), images[2].ID()}, queueIDsOf(sess),
		"会话内部队列仍应完成排序与防连续对调")
}

// 改筛选：读盘只刷新会话已知图片的元数据，成员由聚合按新筛选条件判定；
// 换轮后可撤销，恢复到原筛选条件、队列与进度。
func TestService_UpdateFilter_RefreshesMetadataAndCanUndo(t *testing.T) {
	ctx := context.Background()
	rootDir := t.TempDir()
	rel := testutil.RelIn(t, rootDir, "test.jpg")

	img := image.New(scalar.ToID("img:1"), "test.jpg", rel, scalar.ToID("d1"), 100, time.Now(),
		metadata.NewXMPData(0, "", time.Time{}, ""), 10, 10)
	originalFilter := &shared.ImageFilters{Rating: []int{0}}
	sess := New(scalar.ToID("s1"), scalar.ToID("d1"), originalFilter, 10, []*image.Image{img}, image.NewFilterBuilder())

	metaRepo := NewFakeMetadataRepo()
	imgRepo := &FakeImageRepo{
		MetaRepo: metaRepo,
		BaseDir:  rootDir,
		Images:   map[relpath.Path]*image.Image{rel: img},
	}
	// 磁盘上的 XMP 已被外部改为评分 5
	require.NoError(t, metaRepo.Write(rel.Abs(rootDir), metadata.NewXMPData(5, "", time.Time{}, "")))

	sessionRepo := &FakeSessionRepo{Sessions: map[scalar.ID]*Session{sess.ID(): sess}}
	saved := &mockSessionSaved{ids: make(chan scalar.ID, 4)}
	svc, cleanup := newNextRoundTestService(t, sessionRepo, metaRepo, imgRepo, saved)
	defer cleanup()

	require.NoError(t, svc.Update(ctx, sess.ID(), WithFilter(&shared.ImageFilters{Rating: []int{5}})))

	assert.Equal(t, []int{5}, sess.Filter().Rating, "筛选条件应更新为新条件")
	require.Equal(t, 1, sess.CurrentRound(), "应开启新一轮")
	require.Equal(t, 1, sess.CurrentSize(), "新队列应包含刷新后匹配新条件的图片")
	assert.Equal(t, 5, sess.images[sess.queue[0]].Rating(), "读盘刷新了会话已知图片的元数据")

	// 撤销后恢复原筛选条件、队列与进度
	require.NoError(t, svc.Undo(ctx, sess.ID()))
	assert.Equal(t, originalFilter, sess.Filter())
	assert.Equal(t, 0, sess.CurrentRound())
	assert.Equal(t, 1, sess.CurrentSize())
	assert.Equal(t, img.ID(), sess.images[sess.queue[0]].ID())
}

// 自动推进的候选集是当前轮 Keep 图片；已提交图片的标记结果不应因评分变化
// 而在换轮时丢失，仍应留在下一轮。
func TestSession_TryAdvanceRound_KeepsCommittedKeepImage(t *testing.T) {
	images := createTestImagesWithRatings(t, []int{1, 2})
	filter := &shared.ImageFilters{Rating: []int{1, 2}}
	sess := New(scalar.ToID("s1"), scalar.ToID("d1"), filter, 0, images, image.NewFilterBuilder())

	for _, img := range images {
		require.NoError(t, sess.MarkImage(img.ID(), shared.ImageActionKeep))
	}
	require.Equal(t, 1, sess.CurrentRound())
	require.Equal(t, 2, sess.CurrentSize())

	// 模拟提交把 images[0] 的评分改写为不再匹配筛选条件
	sess.MarkCommitted(images[0].ID())
	sess.images[0] = image.New(
		images[0].ID(), images[0].Filename(), images[0].RelPath(), images[0].DirectoryID(),
		images[0].Size(), images[0].ModTime(), metadata.NewXMPData(9, "", time.Time{}, ""),
		images[0].Width(), images[0].Height(),
	)

	for _, id := range []scalar.ID{images[0].ID(), images[1].ID()} {
		require.NoError(t, sess.MarkImage(id, shared.ImageActionKeep))
	}

	assert.Equal(t, 2, sess.CurrentRound())
	assert.Equal(t, 2, sess.CurrentSize(), "已提交图片应继续属于下一轮")
}

// 「本轮是否走完」的算术由 remainingInRound/currentRoundDone 单点定义，Stats 与自动推进共用。
func TestSession_CurrentRoundDone_SharedWithStats(t *testing.T) {
	sess := setupTestSession(t, 3, 10)
	assert.False(t, sess.currentRoundDone())
	assert.Equal(t, 3, sess.Stats().CurrentRoundRemaining)

	for _, id := range idsOf(sess.knownImages()) {
		require.NoError(t, sess.MarkImage(id, shared.ImageActionReject))
	}

	assert.True(t, sess.currentRoundDone())
	assert.Equal(t, 0, sess.Stats().CurrentRoundRemaining)
	assert.True(t, sess.Stats().IsCompleted)
}

func newNextRoundTestService(t *testing.T, sessionRepo *FakeSessionRepo, metaRepo *FakeMetadataRepo, imgRepo *FakeImageRepo, saved *mockSessionSaved) (*Service, func()) {
	t.Helper()
	svc, cleanup := NewService(
		sessionRepo,
		metaRepo,
		imgRepo,
		&mockFileChangedSub{events: make(chan *directory.FileChangedEvent, 1)},
		&mockMetadataUpdatedPub{events: make(chan *shared.MetadataUpdatedEvent, 1)},
		&FakeDirectoryResolver{},
		zap.NewNop(),
		saved,
		imgRepo.BaseDir,
		image.NewFilterBuilder(),
		&FakeHookRunner{},
		NullSampleRecorder{},
	)
	return svc, cleanup
}

func idsOf(images []*image.Image) []scalar.ID {
	ids := make([]scalar.ID, len(images))
	for i, img := range images {
		ids[i] = img.ID()
	}
	return ids
}
