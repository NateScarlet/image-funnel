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
)

type slowProcessor struct{}

func (slowProcessor) Process(_ context.Context, _ string, _ Spec, w io.Writer) error {
	time.Sleep(5 * time.Millisecond)
	_, err := w.Write([]byte("out"))
	return err
}

func (slowProcessor) Meta(_ context.Context, _ string) (*shared.ImageMeta, error) {
	return &shared.ImageMeta{Width: 1024, Height: 768}, nil
}

func TestAcquireReportsEncodeTiming(t *testing.T) {
	c := NewTranscodeCoordinator(&mockCache{files: make(map[string]File)}, slowProcessor{}, newFakeQueue(), EncoderMagick, nil)
	stop := startWorkers(t, c)
	defer stop()

	src := filepath.Join(t.TempDir(), "a.png")
	require.NoError(t, os.WriteFile(src, []byte("image"), 0o644))
	spec, err := NewSpec(256, ImageFormatWebP)
	require.NoError(t, err)

	_, timing, err := c.Acquire(context.Background(), src, spec, PrioHigh)
	require.NoError(t, err)
	assert.False(t, timing.CacheHit)
	assert.GreaterOrEqual(t, timing.EncodeDuration, 5*time.Millisecond,
		"编码耗时必须来自真实 Process 调用")
}

func TestCachedVariantReportsCacheHit(t *testing.T) {
	c := NewTranscodeCoordinator(&mockCache{files: make(map[string]File)}, slowProcessor{}, newFakeQueue(), EncoderMagick, nil)

	src := filepath.Join(t.TempDir(), "a.png")
	require.NoError(t, os.WriteFile(src, []byte("image"), 0o644))
	spec, err := NewSpec(256, ImageFormatWebP)
	require.NoError(t, err)

	_, timing, err := c.CachedVariant(context.Background(), src, spec)
	require.NoError(t, err)
	assert.False(t, timing.CacheHit, "未命中也必须给出明确状态，不能只返回 nil 文件")

	cache := c.cache.(*mockCache)
	key, err := VariantKey(src, spec, EncoderMagick)
	require.NoError(t, err)
	cache.files[key] = &mockFile{data: []byte("cached")}

	_, timing, err = c.CachedVariant(context.Background(), src, spec)
	require.NoError(t, err)
	assert.True(t, timing.CacheHit)
}
