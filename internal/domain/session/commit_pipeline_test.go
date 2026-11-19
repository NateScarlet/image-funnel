package session

import (
	"context"
	"errors"
	"fmt"
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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// #region 测试脚手架

type pipelineTestEnv struct {
	svc      *Service
	scanner  *FakeImageRepo
	filterFB *image.FilterBuilder
	cleanup  func()
}

// setupPipelineEnv 组装一个使用指定提交后流水线的 Service，采样与钩子的依赖全部显式注入
func setupPipelineEnv(t *testing.T, pipeline CommitPipeline, tempDir string) pipelineTestEnv {
	t.Helper()

	fakeMeta := NewFakeMetadataRepo()
	fileChangedSub := &mockFileChangedSub{events: make(chan *directory.FileChangedEvent, 10)}
	metadataUpdatedPub := &mockMetadataUpdatedPub{events: make(chan *shared.MetadataUpdatedEvent, 10)}
	topic, cleanupTopic := pubsub.NewInMemoryTopic[scalar.ID]()
	scanner := &FakeImageRepo{MetaRepo: fakeMeta, BaseDir: tempDir, Images: make(map[relpath.Path]*image.Image)}
	filterFB := image.NewFilterBuilder()

	svc, cleanupSvc := NewService(
		NewFakeSessionRepo(), fakeMeta, scanner, fileChangedSub, metadataUpdatedPub,
		&FakeDirectoryResolver{}, zap.NewNop(), topic, tempDir, filterFB, pipeline,
	)
	return pipelineTestEnv{
		svc:      svc,
		scanner:  scanner,
		filterFB: filterFB,
		cleanup: func() {
			cleanupSvc()
			cleanupTopic()
		},
	}
}

// addMarkedImage 在临时目录落一个占位文件并把它作为一条已标记决策加入会话
func addMarkedImage(t *testing.T, env pipelineTestEnv, tempDir, id, name string, currentRating int, action shared.ImageAction) *image.Image {
	t.Helper()
	path := filepath.Join(tempDir, name)
	require.NoError(t, os.WriteFile(path, []byte("original-"+name), 0o644))

	img := image.New(
		scalar.ToID(id), name, testutil.Rel(t, name), scalar.ToID("d1"),
		100, time.Now(), metadata.NewXMPData(currentRating, "", time.Time{}, ""), 100, 100,
	)
	env.scanner.Images[img.RelPath()] = img
	return img
}

func sessionOf(env pipelineTestEnv, images ...*image.Image) *Session {
	return New(scalar.ToID("s1"), scalar.ToID("d1"), &shared.ImageFilters{Rating: []int{0}}, 10, images, env.filterFB)
}

func waitFor(t *testing.T, ch <-chan struct{}, msg string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal(msg)
	}
}

// #endregion

func TestCommit_WhenArchiveEnabled_ReturnsBeforeSamplingAndHook(t *testing.T) {
	tempDir := t.TempDir()

	entered := make(chan struct{})
	release := make(chan struct{})
	recorder := &FakeSampleRecorder{OnRecord: func(context.Context, *Sample) error {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		return nil
	}}
	hookFired := make(chan struct{})
	hook := &FakeHookRunner{OnCommitSessionFunc: func(relpath.Path) error {
		close(hookFired)
		return nil
	}}
	pipeline := NewAsyncCommitPipeline(context.Background(), recorder, hook, &FakeDirectoryResolver{}, zap.NewNop())
	env := setupPipelineEnv(t, pipeline, tempDir)
	defer env.cleanup()

	img := addMarkedImage(t, env, tempDir, "1", "test1.jpg", 0, shared.ImageActionKeep)
	sess := sessionOf(env, img)
	require.NoError(t, sess.MarkImage(img.ID(), shared.ImageActionKeep))

	type commitResult struct {
		written int
		matched int
		err     error
	}
	result := make(chan commitResult, 1)
	go func() {
		w, m, err := env.svc.Commit(context.Background(), sess, &shared.WriteActions{KeepRating: ptr(5)})
		result <- commitResult{w, m, err}
	}()

	select {
	case got := <-result:
		require.NoError(t, got.err)
		assert.Equal(t, 1, got.written)
		assert.Equal(t, 1, got.matched)
	case <-time.After(2 * time.Second):
		t.Fatal("Commit blocked waiting for sampling to complete")
	}

	// 后台采样已开始，但被测试阻塞；此时钩子绝不能提前触发
	waitFor(t, entered, "sampling never started in background")
	select {
	case <-hookFired:
		t.Fatal("hook fired before sampling phase completed")
	default:
	}

	close(release)
	waitFor(t, hookFired, "hook never fired after sampling phase completed")
}

func TestCommit_SyncSideEffectsCompleteBeforeReturning(t *testing.T) {
	tempDir := t.TempDir()
	file := filepath.Join(tempDir, "test1.jpg")
	require.NoError(t, os.WriteFile(file, []byte("fake"), 0o644))

	release := make(chan struct{})
	recorder := &FakeSampleRecorder{OnRecord: func(context.Context, *Sample) error {
		<-release
		return nil
	}}
	hookDone := make(chan struct{})
	hook := &FakeHookRunner{OnCommitSessionFunc: func(relpath.Path) error {
		close(hookDone)
		return nil
	}}
	pipeline := NewAsyncCommitPipeline(context.Background(), recorder, hook, &FakeDirectoryResolver{}, zap.NewNop())

	fakeMeta := NewFakeMetadataRepo()
	saved := &mockSessionSaved{ids: make(chan scalar.ID, 1)}
	metadataUpdated := &mockMetadataUpdatedPub{events: make(chan *shared.MetadataUpdatedEvent, 1)}
	scanner := &FakeImageRepo{MetaRepo: fakeMeta, BaseDir: tempDir, Images: make(map[relpath.Path]*image.Image)}
	filterFB := image.NewFilterBuilder()
	svc, cleanupSvc := NewService(
		NewFakeSessionRepo(), fakeMeta, scanner,
		&mockFileChangedSub{events: make(chan *directory.FileChangedEvent, 1)},
		metadataUpdated, &FakeDirectoryResolver{}, zap.NewNop(), saved, tempDir, filterFB, pipeline,
	)
	defer cleanupSvc()

	img := image.New(
		scalar.ToID("1"), "test1.jpg", testutil.Rel(t, "test1.jpg"), scalar.ToID("d1"),
		100, time.Now(), metadata.NewXMPData(0, "", time.Time{}, ""), 100, 100,
	)
	scanner.Images[img.RelPath()] = img
	sess := New(scalar.ToID("s1"), scalar.ToID("d1"), &shared.ImageFilters{Rating: []int{0}}, 10, []*image.Image{img}, filterFB)
	require.NoError(t, sess.MarkImage(img.ID(), shared.ImageActionKeep))

	written, matched, err := svc.Commit(context.Background(), sess, &shared.WriteActions{KeepRating: ptr(5)})
	require.NoError(t, err)
	assert.Equal(t, 1, written)
	assert.Equal(t, 1, matched)
	assert.Equal(t, 5, fakeMeta.Data[file].Rating(), "XMP 应在 Commit 返回前落盘")
	assert.Len(t, metadataUpdated.events, 1, "元数据更新事件应在 Commit 返回前发布")
	assert.Len(t, saved.ids, 1, "session-saved 应在 Commit 返回前发布")
	select {
	case <-hookDone:
		t.Fatal("采样仍被阻塞时钩子不应触发")
	default:
	}

	close(release)
	waitFor(t, hookDone, "hook never fired after sampling completed")
}

func TestCommit_WhenArchiveEnabled_SamplingReadsSourceBeforeHookRemovesIt(t *testing.T) {
	tempDir := t.TempDir()

	file := filepath.Join(tempDir, "test1.jpg")
	require.NoError(t, os.WriteFile(file, []byte("source-before-hook"), 0o644))
	img := image.New(
		scalar.ToID("1"), "test1.jpg", testutil.Rel(t, "test1.jpg"), scalar.ToID("d1"),
		100, time.Now(), metadata.NewXMPData(0, "", time.Time{}, ""), 100, 100,
	)

	entered := make(chan struct{})
	release := make(chan struct{})
	var sampledContent string
	var contentMu sync.Mutex
	recorder := &FakeSampleRecorder{OnRecord: func(_ context.Context, s *Sample) error {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		// 钩子已在采样之后执行；这里读到的仍是原图内容
		data, err := os.ReadFile(s.SourcePath)
		if err != nil {
			return err
		}
		contentMu.Lock()
		sampledContent = string(data)
		contentMu.Unlock()
		return nil
	}}
	hookDone := make(chan struct{})
	hook := &FakeHookRunner{OnCommitSessionFunc: func(relpath.Path) error {
		// 钩子模拟 retention：把源图移走/删除
		require.NoError(t, os.Remove(file))
		close(hookDone)
		return nil
	}}
	pipeline := NewAsyncCommitPipeline(context.Background(), recorder, hook, &FakeDirectoryResolver{}, zap.NewNop())
	env := setupPipelineEnv(t, pipeline, tempDir)
	defer env.cleanup()

	env.scanner.Images[img.RelPath()] = img
	sess := sessionOf(env, img)
	require.NoError(t, sess.MarkImage(img.ID(), shared.ImageActionKeep))
	_, _, err := env.svc.Commit(context.Background(), sess, &shared.WriteActions{KeepRating: ptr(5)})
	require.NoError(t, err)

	waitFor(t, entered, "sampling never started")
	close(release)
	waitFor(t, hookDone, "hook never fired")

	contentMu.Lock()
	defer contentMu.Unlock()
	assert.Equal(t, "source-before-hook", sampledContent)
}

func TestCommit_WhenArchiveEnabled_SampleFailureIsLoggedButDoesNotStopOthersOrHook(t *testing.T) {
	tempDir := t.TempDir()
	core, logs := observer.New(zap.ErrorLevel)

	hookFired := make(chan struct{})
	hook := &FakeHookRunner{OnCommitSessionFunc: func(relpath.Path) error {
		close(hookFired)
		return nil
	}}
	var recordedMu sync.Mutex
	var recorded []scalar.ID
	recorder := &FakeSampleRecorder{OnRecord: func(_ context.Context, s *Sample) error {
		if s.ImageID == scalar.ToID("1") {
			return errors.New("archive failed")
		}
		recordedMu.Lock()
		recorded = append(recorded, s.ImageID)
		recordedMu.Unlock()
		return nil
	}}
	pipeline := NewAsyncCommitPipeline(context.Background(), recorder, hook, &FakeDirectoryResolver{}, zap.New(core))
	env := setupPipelineEnv(t, pipeline, tempDir)
	defer env.cleanup()

	img1 := addMarkedImage(t, env, tempDir, "1", "test1.jpg", 0, shared.ImageActionKeep)
	img2 := addMarkedImage(t, env, tempDir, "2", "test2.jpg", 0, shared.ImageActionReject)
	sess := sessionOf(env, img1, img2)
	require.NoError(t, sess.MarkImage(img1.ID(), shared.ImageActionKeep))
	require.NoError(t, sess.MarkImage(img2.ID(), shared.ImageActionReject))

	_, _, err := env.svc.Commit(context.Background(), sess, &shared.WriteActions{KeepRating: ptr(5), RejectRating: ptr(-1)})
	require.NoError(t, err, "单条采样失败不应进入 Commit 返回值")
	waitFor(t, hookFired, "hook never fired after a sample failure")

	recordedMu.Lock()
	defer recordedMu.Unlock()
	assert.Equal(t, []scalar.ID{scalar.ToID("2")}, recorded, "失败样本之后的样本仍应继续归档")
	entries := logs.FilterMessage("failed to archive training sample").All()
	require.Len(t, entries, 1, "失败应有 zap 日志记录")
	assert.Contains(t, fmt.Sprint(entries[0].ContextMap()["error"]), "archive failed")
}

func TestCommit_WhenArchiveDisabled_HookFiresSynchronouslyWithoutBackgroundTask(t *testing.T) {
	tempDir := t.TempDir()

	var hookCalls int
	hook := &FakeHookRunner{OnCommitSessionFunc: func(relpath.Path) error {
		hookCalls++
		return nil
	}}
	pipeline := NewNullCommitPipeline(context.Background(), NullSampleRecorder{}, hook, &FakeDirectoryResolver{}, zap.NewNop())
	env := setupPipelineEnv(t, pipeline, tempDir)
	defer env.cleanup()

	img := addMarkedImage(t, env, tempDir, "1", "test1.jpg", 0, shared.ImageActionKeep)
	sess := sessionOf(env, img)
	require.NoError(t, sess.MarkImage(img.ID(), shared.ImageActionKeep))

	_, _, err := env.svc.Commit(context.Background(), sess, &shared.WriteActions{KeepRating: ptr(5)})
	require.NoError(t, err)
	// 未开启归档时不创建后台任务：钩子在 Commit 返回前已同步触发（钩子自身的 fire-and-forget 语义不变）
	assert.Equal(t, 1, hookCalls)
}

func TestCommit_WhenArchiveEnabled_UsesServerHeldContext(t *testing.T) {
	tempDir := t.TempDir()
	serverCtx, cancelServer := context.WithCancel(context.Background())
	defer cancelServer()

	seenCtxErr := make(chan error, 1)
	recorder := &FakeSampleRecorder{OnRecord: func(ctx context.Context, _ *Sample) error {
		seenCtxErr <- ctx.Err()
		return nil
	}}
	hookFired := make(chan struct{})
	hook := &FakeHookRunner{OnCommitSessionFunc: func(relpath.Path) error {
		close(hookFired)
		return nil
	}}
	pipeline := NewAsyncCommitPipeline(serverCtx, recorder, hook, &FakeDirectoryResolver{}, zap.NewNop())
	env := setupPipelineEnv(t, pipeline, tempDir)
	defer env.cleanup()

	img := addMarkedImage(t, env, tempDir, "1", "test1.jpg", 0, shared.ImageActionKeep)
	sess := sessionOf(env, img)
	require.NoError(t, sess.MarkImage(img.ID(), shared.ImageActionKeep))

	// 请求上下文在提交前已结束，后台任务仍应使用服务端持有的 ctx
	reqCtx, cancelReq := context.WithCancel(context.Background())
	cancelReq()
	_, _, err := env.svc.Commit(reqCtx, sess, &shared.WriteActions{KeepRating: ptr(5)})
	require.NoError(t, err)
	waitFor(t, hookFired, "hook never fired")
	assert.NoError(t, <-seenCtxErr, "后台任务应拿到服务端 ctx，而不是已取消的请求 ctx")

	// 服务端 ctx 取消后登记的后台任务不再执行采样与钩子
	cancelServer()
	var secondHook atomic.Bool
	hook.OnCommitSessionFunc = func(relpath.Path) error {
		secondHook.Store(true)
		return nil
	}
	var secondRecorded atomic.Bool
	recorder.OnRecord = func(context.Context, *Sample) error {
		secondRecorded.Store(true)
		return nil
	}
	_, _, err = env.svc.Commit(context.Background(), sess, &shared.WriteActions{KeepRating: ptr(5)})
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)
	assert.False(t, secondRecorded.Load(), "服务端 ctx 取消后不应再采样")
	assert.False(t, secondHook.Load(), "服务端 ctx 取消后不应再触发钩子")
}
