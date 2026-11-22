package image

import (
	"bytes"
	"context"
	"errors"
	"io"
	"iter"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"main/internal/infrastructure/inmem"
	"main/internal/shared"
	"main/internal/shared/imageversion"
	"main/internal/shared/transcode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockFile struct {
	data []byte
}

func (f *mockFile) Open() (io.ReadSeekCloser, error) {
	return &seeker{Reader: bytes.NewReader(f.data)}, nil
}

type mockCache struct {
	mu     sync.Mutex
	files  map[string]transcode.File
	misses int
}

func (c *mockCache) Lookup(_ context.Context, key string) (transcode.File, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if file, ok := c.files[key]; ok {
		return file, nil
	}
	c.misses++
	return nil, nil
}

func (c *mockCache) Save(_ context.Context, key string, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.files[key] = &mockFile{data: data}
	return nil
}

type fakeProcessor struct {
	mu     sync.Mutex
	calls  []fakeProcessCall
	result []byte
	err    error
}

type fakeProcessCall struct {
	srcPath string
	width   int
	format  transcode.ImageFormat
}

func (p *fakeProcessor) Process(_ context.Context, srcPath string, spec transcode.Spec, w io.Writer) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, fakeProcessCall{srcPath, spec.Width(), spec.Format()})
	if p.err != nil {
		return p.err
	}
	_, writeErr := w.Write(p.result)
	return writeErr
}

func (p *fakeProcessor) Meta(_ context.Context, _ string) (*shared.ImageMeta, error) {
	return &shared.ImageMeta{Width: 3840, Height: 2160}, nil
}

// startWorkers 启动 n 个通用 worker 与 1 个高优先级专职 worker，返回停止函数
func startWorkers(t *testing.T, c *TranscodeCoordinator) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	c.StartWorkers(ctx, 3)
	return cancel
}

// fakeQueue 测试用最小队列：pending 通道连接 Enqueue 与 Consume，
// 让 coordinator 启动的真实 worker 循环消费任务，链路闭合
type fakeQueue struct {
	pending chan *fakeJob
}

func newFakeQueue() *fakeQueue {
	return &fakeQueue{pending: make(chan *fakeJob, 64)}
}

func (q *fakeQueue) Enqueue(_ context.Context, item transcode.Item) (transcode.Waiter, error) {
	w := &fakeWaiter{done: make(chan struct{}), timing: item.Timing()}
	job := &fakeJob{item: item, waiter: w}
	q.pending <- job
	return w, nil
}

func (q *fakeQueue) Consume(ctx context.Context, _ transcode.Prio) iter.Seq2[transcode.Job, error] {
	return func(yield func(transcode.Job, error) bool) {
		for {
			select {
			case <-ctx.Done():
				return
			case job := <-q.pending:
				if !yield(job, nil) {
					return
				}
			}
		}
	}
}

type fakeJob struct {
	item   transcode.Item
	waiter *fakeWaiter
}

func (j *fakeJob) Item() transcode.Item { return j.item }
func (j *fakeJob) Resolve(file transcode.File, err error) {
	j.waiter.mu.Lock()
	j.waiter.file = file
	j.waiter.err = err
	close(j.waiter.done)
	j.waiter.mu.Unlock()
}

type fakeWaiter struct {
	done   chan struct{}
	mu     sync.Mutex
	file   transcode.File
	err    error
	timing *transcode.VariantTiming
}

func (w *fakeWaiter) Wait(ctx context.Context) (transcode.File, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-w.done:
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file, w.err
}

func (w *fakeWaiter) Timing() *transcode.VariantTiming { return w.timing }

func newTestCoordinator(processor Processor) *TranscodeCoordinator {
	cache := &mockCache{files: make(map[string]transcode.File)}
	// 源图 2048 宽、JPEG；Accept 接受 jpeg 时走缩放转码，只接受 webp 时走源图不可接受转码
	return NewTranscodeCoordinator(cache, processor, newFakeQueue(),
		&stubSourceProbe{width: 2048, contentType: "image/jpeg"}, EncoderMagick, nil)
}

func TestVariant_CacheHitBypassesQueue(t *testing.T) {
	processor := &fakeProcessor{result: []byte("out")}
	c := newTestCoordinator(processor)
	stop := startWorkers(t, c)
	defer stop()

	src := writeVariantSource(t)
	in := VariantInput{
		AbsPath: src, Width: 1024, Format: transcode.ImageFormatAVIF,
		Accept: "image/jpeg, image/avif", Priority: transcode.PrioLow,
	}

	// 预热进缓存
	prew, err := c.Variant(context.Background(), in)
	require.NoError(t, err)
	require.NotNil(t, prew.File)

	// 第二次请求应命中缓存
	in.Priority = transcode.PrioHigh
	again, err := c.Variant(context.Background(), in)
	require.NoError(t, err)
	require.NotNil(t, again.File)

	assert.Equal(t, ResponseServeCached, again.Kind)
	assert.True(t, again.CacheHit)
	assert.Equal(t, 1, len(processor.calls), "processor should only run once")
}

func TestVariant_ProcessError(t *testing.T) {
	processor := &fakeProcessor{err: errors.New("boom")}
	c := newTestCoordinator(processor)
	stop := startWorkers(t, c)
	defer stop()

	src := writeVariantSource(t)
	result, err := c.Variant(context.Background(), VariantInput{
		AbsPath: src, Width: 1024, Format: transcode.ImageFormatWebP,
		Accept: "image/jpeg, image/webp", Priority: transcode.PrioHigh,
	})
	assert.Nil(t, result.File)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

func TestVariant_MissingSource(t *testing.T) {
	processor := &fakeProcessor{result: []byte("out")}
	c := newTestCoordinator(processor)
	stop := startWorkers(t, c)
	defer stop()

	result, err := c.Variant(context.Background(), VariantInput{
		AbsPath: filepath.Join(t.TempDir(), "missing.png"), Width: 1024,
		Format: transcode.ImageFormatWebP, Accept: "image/webp", Priority: transcode.PrioHigh,
	})
	assert.Nil(t, result.File)
	require.Error(t, err)
	assert.True(t, errors.Is(err, os.ErrNotExist), "error should wrap os.ErrNotExist, got %v", err)
}

func TestVariant_ZeroWidthWebPStillTranscodes(t *testing.T) {
	// width=0 且格式为 WebP 时也必须真正转码：源图可能不是 WebP，
	// 直接返回源文件会把非 WebP 字节标成 image/webp
	processor := &fakeProcessor{result: []byte("out")}
	c := newTestCoordinator(processor)
	stop := startWorkers(t, c)
	defer stop()

	src := writeVariantSource(t)
	result, err := c.Variant(context.Background(), VariantInput{
		AbsPath: src, Width: 0, Format: transcode.ImageFormatWebP,
		Accept: "image/webp", Priority: transcode.PrioHigh,
	})
	require.NoError(t, err)
	require.NotNil(t, result.File)

	assert.Equal(t, ResponseTranscode, result.Kind)
	assert.Equal(t, 1, len(processor.calls), "zero-width webp must invoke processor")
}

func TestVariantKey_StableAndDistinct(t *testing.T) {
	src := filepath.Join(t.TempDir(), "a.png")
	require.NoError(t, os.WriteFile(src, []byte("image"), 0o644))

	specA, err := transcode.NewSpec(1024, transcode.ImageFormatAVIF)
	require.NoError(t, err)
	specB, err := transcode.NewSpec(2048, transcode.ImageFormatAVIF)
	require.NoError(t, err)

	keyA1, err := VariantKey(src, specA, EncoderSVTAV1)
	require.NoError(t, err)
	keyA2, err := VariantKey(src, specA, EncoderSVTAV1)
	require.NoError(t, err)
	keyB, err := VariantKey(src, specB, EncoderSVTAV1)
	require.NoError(t, err)
	keyMagick, err := VariantKey(src, specA, EncoderMagick)
	require.NoError(t, err)

	assert.Equal(t, keyA1, keyA2, "same inputs must produce same key")
	assert.NotEqual(t, keyA1, keyB, "different width must produce different key")
	assert.NotEqual(t, keyA1, keyMagick, "different encoder must produce different key")
}

func TestVariantKey_TouchFileChangesKey(t *testing.T) {
	// 变体身份包含源图修改时间与大小：文件变更后 key 变化（陈旧缓存自然失效）
	dir := t.TempDir()
	src := filepath.Join(dir, "a.png")
	require.NoError(t, os.WriteFile(src, []byte("image"), 0o644))

	spec, err := transcode.NewSpec(1024, transcode.ImageFormatWebP)
	require.NoError(t, err)

	key1, err := VariantKey(src, spec, EncoderMagick)
	require.NoError(t, err)

	future := time.Now().Add(2 * time.Hour)
	require.NoError(t, os.Chtimes(src, future, future))

	key2, err := VariantKey(src, spec, EncoderMagick)
	require.NoError(t, err)

	assert.NotEqual(t, key1, key2, "modTime change must change variant key")
}

// 变体 key 的 mtime/size 分量必须来自共享图片版本的纳秒视图：
// 纳秒视图变化则 key 变化，无关输入不变则 key 不变
func TestVariantKey_UsesSharedNanoVersion(t *testing.T) {
	src := filepath.Join(t.TempDir(), "a.png")
	require.NoError(t, os.WriteFile(src, []byte("image"), 0o644))

	spec, err := transcode.NewSpec(1024, transcode.ImageFormatWebP)
	require.NoError(t, err)

	info, err := os.Stat(src)
	require.NoError(t, err)
	nano1 := imageversion.New(info).Nano()

	key1, err := VariantKey(src, spec, EncoderMagick)
	require.NoError(t, err)
	key2, err := VariantKey(src, spec, EncoderMagick)
	require.NoError(t, err)
	assert.Equal(t, key1, key2, "无关输入不变时 key 必须不变")

	// 仅亚秒级修改时间变化：秒级视图不变，但纳秒视图变化，key 必须随之变化
	future := info.ModTime().Add(500 * time.Millisecond)
	require.NoError(t, os.Chtimes(src, future, future))
	info2, err := os.Stat(src)
	require.NoError(t, err)
	require.NotEqual(t, nano1, imageversion.New(info2).Nano(), "前提：纳秒视图确实变化")

	key3, err := VariantKey(src, spec, EncoderMagick)
	require.NoError(t, err)
	assert.NotEqual(t, key1, key3, "纳秒视图变化必须改变 key")
}

// Variant 把请求 ctx 直接交给队列作为登记生命周期：ctx 结束即撤回需求，
// 未启动的任务不会交给 worker（与 ADR 0002 的「abort 即撤回需求」一致）
func TestVariant_RequestContextWithdrawsDemand(t *testing.T) {
	cache := &mockCache{files: make(map[string]transcode.File)}
	queue := inmem.NewTranscodeQueue()
	c := NewTranscodeCoordinator(cache, &fakeProcessor{result: []byte("out")}, queue,
		&stubSourceProbe{width: 2048, contentType: "image/jpeg"}, EncoderMagick, nil)

	src := writeVariantSource(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 客户端在入队前已放弃请求
	_, err := c.Variant(ctx, VariantInput{
		AbsPath: src, Width: 1024, Format: transcode.ImageFormatWebP,
		Accept: "image/webp", Priority: transcode.PrioLow,
	})
	assert.ErrorIs(t, err, context.Canceled)

	// 撤回先于消费启动，确定性验证该任务不会被认领
	jobs := make(chan transcode.Job, 1)
	go func() {
		for job, err := range queue.Consume(context.Background(), transcode.PrioLow) {
			if err != nil {
				return
			}
			jobs <- job
			return
		}
	}()
	select {
	case job := <-jobs:
		t.Fatalf("withdrawn demand should not be claimed: %v", job.Item().Key())
	case <-time.After(100 * time.Millisecond):
	}
}
