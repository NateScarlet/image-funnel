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

	"main/internal/shared/transcode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #region seam 测试替身

// recordingCache 记录每次 Lookup/Save 的 key，用于断言「一次 key 计算、一次 Lookup、
// 入队与 Save 复用同一 key」。复用 mockFile 作为产物句柄。
type recordingCache struct {
	mu      sync.Mutex
	files   map[string]transcode.File
	lookups []string
	saves   []string
}

func newRecordingCache() *recordingCache {
	return &recordingCache{files: make(map[string]transcode.File)}
}

func (c *recordingCache) Lookup(_ context.Context, key string) (transcode.File, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lookups = append(c.lookups, key)
	return c.files[key], nil
}

func (c *recordingCache) Save(_ context.Context, key string, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.saves = append(c.saves, key)
	c.files[key] = &mockFile{data: data}
	return nil
}

func (c *recordingCache) lookupKeys() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.lookups...)
}

func (c *recordingCache) saveKeys() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.saves...)
}

// recordingQueue 记录 Enqueue 的 key，并把任务交给真实 worker 链路（由 StartWorkers 启动），
// 使 Save 由生产代码 runJob 完成而非测试替身代劳。
type recordingQueue struct {
	mu       sync.Mutex
	enqueued []string
	pending  chan transcode.Job
}

func newRecordingQueue() *recordingQueue {
	return &recordingQueue{pending: make(chan transcode.Job, 16)}
}

func (q *recordingQueue) Enqueue(_ context.Context, item transcode.Item) (transcode.Waiter, error) {
	w := &recordingWaiter{done: make(chan struct{}), timing: item.Timing()}
	q.mu.Lock()
	q.enqueued = append(q.enqueued, item.Key())
	q.mu.Unlock()
	q.pending <- &recordingJob{item: item, waiter: w}
	return w, nil
}

func (q *recordingQueue) Consume(ctx context.Context, _ transcode.Prio) iter.Seq2[transcode.Job, error] {
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

func (q *recordingQueue) enqueueKeys() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]string(nil), q.enqueued...)
}

type recordingJob struct {
	item   transcode.Item
	waiter *recordingWaiter
}

func (j *recordingJob) Item() transcode.Item { return j.item }

func (j *recordingJob) Resolve(file transcode.File, err error) {
	j.waiter.mu.Lock()
	j.waiter.file = file
	j.waiter.err = err
	close(j.waiter.done)
	j.waiter.mu.Unlock()
}

type recordingWaiter struct {
	done   chan struct{}
	mu     sync.Mutex
	file   transcode.File
	err    error
	timing *transcode.VariantTiming
}

func (w *recordingWaiter) Wait(ctx context.Context) (transcode.File, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-w.done:
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file, w.err
}

func (w *recordingWaiter) Timing() *transcode.VariantTiming { return w.timing }

// stubSourceProbe 返回固定宽度与 MIME 的源图，并记录 Open/Probe 次数
type stubSourceProbe struct {
	mu          sync.Mutex
	opens       int
	probes      int
	width       int
	contentType string
}

func (p *stubSourceProbe) Open(string) (Source, error) {
	p.mu.Lock()
	p.opens++
	p.mu.Unlock()
	return p.source()
}

func (p *stubSourceProbe) Probe(context.Context, string) (Source, error) {
	p.mu.Lock()
	p.probes++
	p.mu.Unlock()
	return p.source()
}

func (p *stubSourceProbe) source() (Source, error) {
	return Source{
		Width:       p.width,
		ContentType: p.contentType,
		Reader:      &seeker{Reader: bytes.NewReader([]byte("source-bytes"))},
	}, nil
}

func (p *stubSourceProbe) counts() (int, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.opens, p.probes
}

// #endregion

// newVariantTestCoordinator 构造只注入 Cache/Probe/Queue/Processor 的 seam，
// 不构造签名器、handler 或真实文件之外的 HTTP 依赖。
func newVariantTestCoordinator(cache Cache, queue transcode.Queue, probe SourceProbe, processor Processor) *TranscodeCoordinator {
	return NewTranscodeCoordinator(cache, processor, queue, probe, EncoderMagick, nil)
}

func writeVariantSource(t *testing.T) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), "a.png")
	require.NoError(t, os.WriteFile(src, []byte("image"), 0o644))
	return src
}

// #region 缓存命中

func TestVariant_CacheHitSingleLookupNoProbeNoEnqueue(t *testing.T) {
	// 命中路径：一次 key 计算 + 一次 Lookup，零探源、零入队、零写缓存，且不重定向
	cache := newRecordingCache()
	queue := newRecordingQueue()
	probe := &stubSourceProbe{width: 512, contentType: "image/jpeg"}
	c := newVariantTestCoordinator(cache, queue, probe, &fakeProcessor{result: []byte("encoded")})

	src := writeVariantSource(t)
	spec, err := transcode.NewSpec(1024, transcode.ImageFormatAVIF)
	require.NoError(t, err)
	key, err := VariantKey(src, spec, EncoderMagick)
	require.NoError(t, err)
	require.NoError(t, cache.Save(context.Background(), key, bytes.NewReader([]byte("cached-variant"))))

	result, err := c.Variant(context.Background(), VariantInput{
		AbsPath: src, Width: 1024, Format: transcode.ImageFormatAVIF,
		Accept: "image/avif", Priority: transcode.PrioHigh,
	})
	require.NoError(t, err)

	assert.Equal(t, ResponseServeCached, result.Kind)
	assert.Equal(t, transcode.ImageFormatAVIF, result.Format)
	assert.True(t, result.CacheHit)
	require.NotNil(t, result.File)
	assert.Equal(t, []string{key}, cache.lookupKeys(), "命中只发生一次 Lookup")
	assert.Empty(t, queue.enqueueKeys(), "命中不入队")
	opens, probes := probe.counts()
	assert.Zero(t, opens)
	assert.Zero(t, probes, "命中不探源")
	// 预热写入之外不应有新的 Save
	assert.Len(t, cache.saveKeys(), 1)
}

func TestVariant_CacheHitAboveSourceDoesNotRedirect(t *testing.T) {
	// w>源宽但变体已命中：直接服务缓存，重定向不得发生在命中路径
	cache := newRecordingCache()
	queue := newRecordingQueue()
	probe := &stubSourceProbe{width: 512, contentType: "image/jpeg"}
	c := newVariantTestCoordinator(cache, queue, probe, &fakeProcessor{result: []byte("encoded")})

	src := writeVariantSource(t)
	spec, err := transcode.NewSpec(4096, transcode.ImageFormatAVIF)
	require.NoError(t, err)
	key, err := VariantKey(src, spec, EncoderMagick)
	require.NoError(t, err)
	require.NoError(t, cache.Save(context.Background(), key, bytes.NewReader([]byte("cached-variant"))))

	result, err := c.Variant(context.Background(), VariantInput{
		AbsPath: src, Width: 4096, Format: transcode.ImageFormatAVIF,
		Accept: "image/avif", Priority: transcode.PrioHigh,
	})
	require.NoError(t, err)

	assert.Equal(t, ResponseServeCached, result.Kind)
	assert.Zero(t, result.CanonicalWidth, "命中路径不产生重定向目标")
	_, probes := probe.counts()
	assert.Zero(t, probes)
	assert.Empty(t, queue.enqueueKeys())
}

// #endregion

// #region 未命中与重定向

func TestVariant_CacheMissRedirectsWithoutWriteOrEnqueue(t *testing.T) {
	// 未命中：一次 key 计算 + 一次 Lookup + 一次探源；重定向不写缓存、不入队
	cache := newRecordingCache()
	queue := newRecordingQueue()
	probe := &stubSourceProbe{width: 512, contentType: "image/jpeg"}
	c := newVariantTestCoordinator(cache, queue, probe, &fakeProcessor{result: []byte("encoded")})

	src := writeVariantSource(t)
	spec, err := transcode.NewSpec(4096, transcode.ImageFormatAVIF)
	require.NoError(t, err)
	key, err := VariantKey(src, spec, EncoderMagick)
	require.NoError(t, err)

	result, err := c.Variant(context.Background(), VariantInput{
		AbsPath: src, Width: 4096, Format: transcode.ImageFormatAVIF,
		Accept: "image/avif", Priority: transcode.PrioHigh,
	})
	require.NoError(t, err)

	assert.False(t, result.CacheHit)
	assert.Equal(t, ResponseRedirectCanonicalWidth, result.Kind)
	assert.Equal(t, 512, result.CanonicalWidth)
	assert.Equal(t, []string{key}, cache.lookupKeys(), "未命中只发生一次 Lookup")
	_, probes := probe.counts()
	assert.Equal(t, 1, probes, "未命中只探源一次")
	assert.Empty(t, queue.enqueueKeys(), "重定向不入队")
	assert.Empty(t, cache.saveKeys(), "重定向不写缓存")
}

func TestVariant_FullSizeAcceptableRedirectsRawWithoutLookup(t *testing.T) {
	// w=0 是「全尺寸诉求」，沿用既有规则：不查询变体缓存，探源一次后重定向到 raw
	cache := newRecordingCache()
	queue := newRecordingQueue()
	probe := &stubSourceProbe{width: 2048, contentType: "image/jpeg"}
	c := newVariantTestCoordinator(cache, queue, probe, &fakeProcessor{result: []byte("encoded")})

	src := writeVariantSource(t)
	result, err := c.Variant(context.Background(), VariantInput{
		AbsPath: src, Width: 0, Format: transcode.ImageFormatWebP,
		Accept: "image/jpeg, image/webp", Priority: transcode.PrioHigh,
	})
	require.NoError(t, err)

	assert.Equal(t, ResponseRedirectRaw, result.Kind)
	assert.Empty(t, cache.lookupKeys(), "w=0 不查询变体缓存")
	_, probes := probe.counts()
	assert.Equal(t, 1, probes)
	assert.Empty(t, queue.enqueueKeys())
	assert.Empty(t, cache.saveKeys())
}

// #endregion

// #region 转码

func TestVariant_TranscodeReusesLookupKeyForEnqueueAndSave(t *testing.T) {
	// 转码：Lookup 与入队/Save 各一次且 key 完全相同
	cache := newRecordingCache()
	queue := newRecordingQueue()
	probe := &stubSourceProbe{width: 2048, contentType: "image/jpeg"}
	c := newVariantTestCoordinator(cache, queue, probe, &fakeProcessor{result: []byte("encoded")})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.StartWorkers(ctx, 2)

	src := writeVariantSource(t)
	spec, err := transcode.NewSpec(1024, transcode.ImageFormatWebP)
	require.NoError(t, err)
	key, err := VariantKey(src, spec, EncoderMagick)
	require.NoError(t, err)

	result, err := c.Variant(context.Background(), VariantInput{
		AbsPath: src, Width: 1024, Format: transcode.ImageFormatWebP,
		Accept: "image/jpeg, image/webp", Priority: transcode.PrioHigh,
	})
	require.NoError(t, err)

	assert.Equal(t, ResponseTranscode, result.Kind)
	assert.Equal(t, 1024, result.EffectiveWidth)
	assert.False(t, result.CacheHit)
	require.NotNil(t, result.File)
	assert.GreaterOrEqual(t, result.EncodeDuration, time.Duration(0))

	assert.Equal(t, []string{key}, cache.lookupKeys(), "Lookup 一次")
	assert.Equal(t, []string{key}, queue.enqueueKeys(), "入队 key 与 Lookup key 相同")
	assert.Equal(t, []string{key}, cache.saveKeys(), "Save key 与 Lookup key 相同")
	_, probes := probe.counts()
	assert.Equal(t, 1, probes)
}

func TestVariant_ProcessErrorPropagates(t *testing.T) {
	cache := newRecordingCache()
	queue := newRecordingQueue()
	probe := &stubSourceProbe{width: 2048, contentType: "image/jpeg"}
	c := newVariantTestCoordinator(cache, queue, probe, &fakeProcessor{err: errors.New("boom")})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.StartWorkers(ctx, 2)

	src := writeVariantSource(t)
	_, err := c.Variant(context.Background(), VariantInput{
		AbsPath: src, Width: 1024, Format: transcode.ImageFormatWebP,
		Accept: "image/jpeg, image/webp", Priority: transcode.PrioHigh,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

func TestVariant_MissingSourcePropagatesNotFound(t *testing.T) {
	// 源图不存在时 key 计算阶段即失败：不查缓存、不探源、不入队
	cache := newRecordingCache()
	queue := newRecordingQueue()
	probe := &stubSourceProbe{width: 2048, contentType: "image/jpeg"}
	c := newVariantTestCoordinator(cache, queue, probe, &fakeProcessor{result: []byte("out")})

	_, err := c.Variant(context.Background(), VariantInput{
		AbsPath: filepath.Join(t.TempDir(), "missing.png"), Width: 1024,
		Format: transcode.ImageFormatWebP, Accept: "image/webp", Priority: transcode.PrioHigh,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, os.ErrNotExist)
	assert.Empty(t, cache.lookupKeys())
	assert.Empty(t, queue.enqueueKeys())
	_, probes := probe.counts()
	assert.Zero(t, probes)
}

// #endregion
