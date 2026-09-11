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

	"main/internal/shared"

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
	files  map[string]File
	misses int
}

func (c *mockCache) Lookup(_ context.Context, key string) (File, error) {
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
	quality int
	format  ImageFormat
}

func (p *fakeProcessor) Process(_ context.Context, srcPath string, spec Spec, w io.Writer) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, fakeProcessCall{srcPath, spec.Width(), spec.Quality(), spec.Format()})
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

func (q *fakeQueue) Enqueue(_ context.Context, item TranscodeItem) (TranscodeWaiter, error) {
	w := &fakeWaiter{done: make(chan struct{})}
	job := &fakeJob{item: item, waiter: w}
	q.pending <- job
	return w, nil
}

func (q *fakeQueue) Consume(ctx context.Context, _ Prio) iter.Seq2[TranscodeJob, error] {
	return func(yield func(TranscodeJob, error) bool) {
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
	item   TranscodeItem
	waiter *fakeWaiter
}

func (j *fakeJob) Item() TranscodeItem { return j.item }
func (j *fakeJob) Resolve(file File, err error) {
	j.waiter.mu.Lock()
	j.waiter.file = file
	j.waiter.err = err
	close(j.waiter.done)
	j.waiter.mu.Unlock()
}

type fakeWaiter struct {
	done chan struct{}
	mu   sync.Mutex
	file File
	err  error
}

func (w *fakeWaiter) Wait(ctx context.Context) (File, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-w.done:
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file, w.err
}

func (w *fakeWaiter) Cancel() {}

func newTestCoordinator(processor Processor) *TranscodeCoordinator {
	cache := &mockCache{files: make(map[string]File)}
	return NewTranscodeCoordinator(cache, processor, newFakeQueue(), EncoderMagick, nil)
}

func TestAcquireVariant_CacheHitBypassesQueue(t *testing.T) {
	processor := &fakeProcessor{result: []byte("out")}
	c := newTestCoordinator(processor)
	stop := startWorkers(t, c)
	defer stop()

	src := filepath.Join(t.TempDir(), "a.png")
	require.NoError(t, os.WriteFile(src, []byte("image"), 0o644))

	spec, err := NewSpec(1024, 85, ImageFormatAVIF)
	require.NoError(t, err)

	// 预热进缓存
	prew, err := c.Acquire(context.Background(), src, spec, PrioLow)
	require.NoError(t, err)
	require.NotNil(t, prew)

	// 第二次请求应命中缓存
	again, err := c.Acquire(context.Background(), src, spec, PrioHigh)
	require.NoError(t, err)
	require.NotNil(t, again)

	assert.Equal(t, 1, len(processor.calls), "processor should only run once")
}

func TestAcquireVariant_ProcessError(t *testing.T) {
	processor := &fakeProcessor{err: errors.New("boom")}
	c := newTestCoordinator(processor)
	stop := startWorkers(t, c)
	defer stop()

	src := filepath.Join(t.TempDir(), "a.png")
	require.NoError(t, os.WriteFile(src, []byte("image"), 0o644))

	spec, err := NewSpec(1024, 85, ImageFormatWebP)
	require.NoError(t, err)

	file, err := c.Acquire(context.Background(), src, spec, PrioHigh)
	assert.Nil(t, file)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

func TestAcquireVariant_MissingSource(t *testing.T) {
	processor := &fakeProcessor{result: []byte("out")}
	c := newTestCoordinator(processor)
	stop := startWorkers(t, c)
	defer stop()

	spec, err := NewSpec(1024, 85, ImageFormatWebP)
	require.NoError(t, err)

	file, err := c.Acquire(context.Background(), filepath.Join(t.TempDir(), "missing.png"), spec, PrioHigh)
	assert.Nil(t, file)
	require.Error(t, err)
	assert.True(t, errors.Is(err, os.ErrNotExist), "error should wrap os.ErrNotExist, got %v", err)
}

func TestAcquireVariant_RawPassthrough(t *testing.T) {
	// webp + 无宽 + 无质量 → 直接返回原始文件（现状行为原样搬移）
	processor := &fakeProcessor{result: []byte("out")}
	c := newTestCoordinator(processor)
	stop := startWorkers(t, c)
	defer stop()

	src := filepath.Join(t.TempDir(), "a.jpg")
	require.NoError(t, os.WriteFile(src, []byte("image"), 0o644))

	spec, err := NewSpec(0, 0, ImageFormatWebP)
	require.NoError(t, err)

	file, err := c.Acquire(context.Background(), src, spec, PrioHigh)
	require.NoError(t, err)
	require.NotNil(t, file)

	assert.Empty(t, processor.calls, "raw passthrough must not invoke processor")
}

func TestVariantKey_StableAndDistinct(t *testing.T) {
	src := filepath.Join(t.TempDir(), "a.png")
	require.NoError(t, os.WriteFile(src, []byte("image"), 0o644))

	specA, err := NewSpec(1024, 85, ImageFormatAVIF)
	require.NoError(t, err)
	specB, err := NewSpec(2048, 85, ImageFormatAVIF)
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

	spec, err := NewSpec(1024, 85, ImageFormatWebP)
	require.NoError(t, err)

	key1, err := VariantKey(src, spec, EncoderMagick)
	require.NoError(t, err)

	future := time.Now().Add(2 * time.Hour)
	require.NoError(t, os.Chtimes(src, future, future))

	key2, err := VariantKey(src, spec, EncoderMagick)
	require.NoError(t, err)

	assert.NotEqual(t, key1, key2, "modTime change must change variant key")
}
