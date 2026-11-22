package image

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"main/internal/shared"
	"main/internal/shared/transcode"
)

type slowProcessor struct{}

func (slowProcessor) Process(_ context.Context, _ string, _ transcode.Spec, w io.Writer) error {
	time.Sleep(5 * time.Millisecond)
	_, err := w.Write([]byte("out"))
	return err
}

func (slowProcessor) Meta(_ context.Context, _ string) (*shared.ImageMeta, error) {
	return &shared.ImageMeta{Width: 1024, Height: 768}, nil
}

func TestVariantReportsEncodeTiming(t *testing.T) {
	cache := &mockCache{files: make(map[string]transcode.File)}
	queue := newFakeQueue()
	c := NewTranscodeCoordinator(cache, slowProcessor{}, queue,
		&stubSourceProbe{width: 2048, contentType: "image/jpeg"}, EncoderMagick, nil)
	stop := startWorkers(t, c)
	defer stop()

	src := filepath.Join(t.TempDir(), "a.png")
	require.NoError(t, os.WriteFile(src, []byte("image"), 0o644))

	result, err := c.Variant(context.Background(), VariantInput{
		AbsPath: src, Width: 256, Format: transcode.ImageFormatWebP,
		Accept: "image/jpeg, image/webp", Priority: transcode.PrioHigh,
	})
	require.NoError(t, err)
	assert.False(t, result.CacheHit)
	assert.GreaterOrEqual(t, result.EncodeDuration, 5*time.Millisecond,
		"编码耗时必须来自真实 Process 调用")
}

func TestVariantReportsCacheHit(t *testing.T) {
	cache := &mockCache{files: make(map[string]transcode.File)}
	queue := newFakeQueue()
	c := NewTranscodeCoordinator(cache, slowProcessor{}, queue,
		&stubSourceProbe{width: 2048, contentType: "image/jpeg"}, EncoderMagick, nil)
	stop := startWorkers(t, c)
	defer stop()

	src := filepath.Join(t.TempDir(), "a.png")
	require.NoError(t, os.WriteFile(src, []byte("image"), 0o644))
	in := VariantInput{
		AbsPath: src, Width: 256, Format: transcode.ImageFormatWebP,
		Accept: "image/jpeg, image/webp", Priority: transcode.PrioHigh,
	}

	// 未命中路径必须给出明确状态，不能只返回 nil 文件
	miss, err := c.Variant(context.Background(), in)
	require.NoError(t, err)
	assert.False(t, miss.CacheHit)

	// 预热缓存后命中
	spec, err := transcode.NewSpec(256, transcode.ImageFormatWebP)
	require.NoError(t, err)
	key, err := VariantKey(src, spec, EncoderMagick)
	require.NoError(t, err)
	cache.files[key] = &mockFile{data: []byte("cached")}

	hit, err := c.Variant(context.Background(), in)
	require.NoError(t, err)
	assert.True(t, hit.CacheHit)
}
