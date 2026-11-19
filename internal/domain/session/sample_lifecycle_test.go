package session

import (
	"context"
	"main/internal/domain/directory"
	"main/internal/domain/image"
	"main/internal/domain/metadata"
	"main/internal/domain/relpath"
	"main/internal/pubsub"
	"main/internal/scalar"
	"main/internal/shared"
	"main/internal/testutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// #region Mark 载荷与基准集合

func TestMarkImage_RecordsSampleWithBaselinesFromStateBeforeMark(t *testing.T) {
	sess := setupTestSession(t, 3, 1)
	id0 := sess.images[sess.queue[0]].ID()
	id1 := sess.images[sess.queue[1]].ID()

	require.NoError(t, sess.MarkImage(id0, shared.ImageActionKeep))
	require.NoError(t, sess.MarkImage(id1, shared.ImageActionReject))

	samples := sess.LiveSamples()
	require.Len(t, samples, 2, "每次 Mark 产生一条载荷")

	assert.Equal(t, id0, samples[0].ImageID)
	assert.Empty(t, samples[0].Baselines, "首张图被标记前没有基准")

	assert.Equal(t, id1, samples[1].ImageID)
	require.Len(t, samples[1].Baselines, 1)
	assert.Equal(t, id0, samples[1].Baselines[0].ID, "基准是 Mark 当下会话内的 KEEP 图")
	assert.Equal(t, sess.images[sess.indexByID[id0]].RelPath(), samples[1].Baselines[0].RelPath)
}

func TestMarkImage_CurrentImageNeverInItsOwnBaseline(t *testing.T) {
	sess := setupTestSession(t, 2, 1)
	id0 := sess.images[sess.queue[0]].ID()

	require.NoError(t, sess.MarkImage(id0, shared.ImageActionKeep))
	require.NoError(t, sess.MarkImage(id0, shared.ImageActionReject))

	samples := sess.LiveSamples()
	require.Len(t, samples, 2)
	assert.Empty(t, samples[1].Baselines, "当前图不进自己的基准集合")
}

// #endregion

// #region Undo 同步移除载荷

func TestUndo_RemovesSamplePayloadOfRevertedMark(t *testing.T) {
	sess := setupTestSession(t, 3, 1)
	id0 := sess.images[sess.queue[0]].ID()
	id1 := sess.images[sess.queue[1]].ID()

	require.NoError(t, sess.MarkImage(id0, shared.ImageActionKeep))
	require.NoError(t, sess.MarkImage(id1, shared.ImageActionReject))
	require.Len(t, sess.LiveSamples(), 2)

	require.NoError(t, sess.Undo())
	assert.Len(t, sess.LiveSamples(), 1, "撤销的 Mark 载荷必须同步移除")

	require.NoError(t, sess.Undo())
	assert.Empty(t, sess.LiveSamples())
}

func TestUndo_CrossRoundChainRemovesAllSamplePayloads(t *testing.T) {
	sess := setupTestSession(t, 4, 1)
	ids := make([]scalar.ID, 4)
	for i := range ids {
		ids[i] = sess.images[sess.queue[i]].ID()
	}

	// 首轮全部 KEEP：队列走完且 KEEP 数超过目标后自动推进新一轮
	for _, id := range ids {
		require.NoError(t, sess.MarkImage(id, shared.ImageActionKeep))
	}
	require.Greater(t, sess.CurrentRound(), 0, "应已进入下一轮")
	require.NoError(t, sess.MarkImage(sess.CurrentImage().ID(), shared.ImageActionReject))
	require.Len(t, sess.LiveSamples(), 5)

	for sess.CanUndo() {
		require.NoError(t, sess.Undo())
	}
	assert.Empty(t, sess.LiveSamples(), "换轮闭包连锁撤销时载荷逐条移除")
}

// #endregion

// #region Commit 只提交存活载荷

type sampleLifecycleEnv struct {
	svc      *Service
	repo     *FakeSessionRepo
	recorder *FakeSampleRecorder
	cleanup  func()
}

func setupSampleLifecycleEnv(t *testing.T, tempDir string) sampleLifecycleEnv {
	t.Helper()
	repo := NewFakeSessionRepo()
	fakeMeta := NewFakeMetadataRepo()
	fileChangedSub := &mockFileChangedSub{events: make(chan *directory.FileChangedEvent, 10)}
	metadataUpdatedPub := &mockMetadataUpdatedPub{events: make(chan *shared.MetadataUpdatedEvent, 10)}
	topic, cleanupTopic := pubsub.NewInMemoryTopic[scalar.ID]()
	scanner := &FakeImageRepo{MetaRepo: fakeMeta, BaseDir: tempDir, Images: make(map[relpath.Path]*image.Image)}
	recorder := &FakeSampleRecorder{}
	pipeline := NewNullCommitPipeline(context.Background(), recorder, &FakeHookRunner{}, &FakeDirectoryResolver{}, zap.NewNop())

	svc, cleanupSvc := NewService(
		repo, fakeMeta, scanner, fileChangedSub, metadataUpdatedPub,
		&FakeDirectoryResolver{}, zap.NewNop(), topic, tempDir, image.NewFilterBuilder(), pipeline,
	)
	return sampleLifecycleEnv{
		svc:      svc,
		repo:     repo,
		recorder: recorder,
		cleanup: func() {
			cleanupSvc()
			cleanupTopic()
		},
	}
}

func addLifecycleImage(t *testing.T, env sampleLifecycleEnv, tempDir, id, name string) *image.Image {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, name), []byte("original-"+name), 0o644))
	img := image.New(
		scalar.ToID(id), name, testutil.Rel(t, name), scalar.ToID("d1"),
		100, time.Now(), metadata.NewXMPData(0, "", time.Time{}, ""), 100, 100,
	)
	return img
}

func TestService_Commit_SubmitsOnlySurvivingSamplesAndClearsThem(t *testing.T) {
	tempDir := t.TempDir()
	env := setupSampleLifecycleEnv(t, tempDir)
	defer env.cleanup()

	img0 := addLifecycleImage(t, env, tempDir, "1", "a.jpg")
	img1 := addLifecycleImage(t, env, tempDir, "2", "b.jpg")
	sess := New(scalar.ToID("s1"), scalar.ToID("d1"), &shared.ImageFilters{Rating: []int{0}}, 10, []*image.Image{img0, img1}, image.NewFilterBuilder())
	_, err := env.repo.Create(sess)
	require.NoError(t, err)

	require.NoError(t, env.svc.MarkImage(context.Background(), sess.ID(), img0.ID(), shared.ImageActionKeep))
	require.NoError(t, env.svc.MarkImage(context.Background(), sess.ID(), img1.ID(), shared.ImageActionReject))
	require.NoError(t, env.svc.Undo(context.Background(), sess.ID()), "撤销第二次 Mark")

	_, _, err = env.svc.Commit(context.Background(), sess, &shared.WriteActions{KeepRating: ptr(5), RejectRating: ptr(1)})
	require.NoError(t, err)

	archived := env.recorder.SamplesSnapshot()
	require.Len(t, archived, 1, "只提交会话中存活的载荷")
	assert.Equal(t, img0.ID(), archived[0].ImageID)
	assert.Empty(t, sess.LiveSamples(), "提交后载荷从会话清空，避免重复提交")
}

// #endregion
