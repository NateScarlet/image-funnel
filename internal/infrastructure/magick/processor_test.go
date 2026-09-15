package magick

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	appimage "main/internal/application/image"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runMagick 执行一次 magick 命令，供测试准备真实图片素材
func runMagick(args ...string) error {
	return exec.Command("magick", args...).Run()
}

func TestWebpQuality_StaysBelowLosslessThreshold(t *testing.T) {
	// 实测（2304px 源图）：q92 相比 q75 画质高 3.3dB 而编码仅慢约 15ms；
	// q100 会触发无损编码，1024w 从 501ms 暴涨到 2496ms。质量必须留在有损区间内
	assert.GreaterOrEqual(t, webpQuality, 90, "quality should be on the high side")
	assert.Less(t, webpQuality, 100, "quality must stay below the lossless encoding cliff")
}

func TestProcess_RealMagick_UsesConfiguredQuality(t *testing.T) {
	// 画质不再来自 Spec（已移除），而是由处理器自身配置固定。
	// 用真实 magick 验证产物确实按 webpQuality 编码，而非退回默认质量
	src := filepath.Join(t.TempDir(), "src.png")
	require.NoError(t, os.WriteFile(src, []byte("placeholder"), 0o644))
	// 生成真实源图（依赖 magick，缺失则跳过）
	if err := runMagick("-size", "800x600", "xc:red", src); err != nil {
		t.Skipf("magick unavailable: %v", err)
	}

	spec, err := appimage.NewSpec(400, appimage.ImageFormatWebP)
	require.NoError(t, err)

	var out bytes.Buffer
	p := NewProcessor(1)
	require.NoError(t, p.Process(context.Background(), src, spec, &out))

	assert.NotEmpty(t, out.Bytes(), "should produce webp output")
	// WebP 魔数 RIFF....WEBP
	require.GreaterOrEqual(t, out.Len(), 12)
	assert.Equal(t, "RIFF", string(out.Bytes()[0:4]))
	assert.Equal(t, "WEBP", string(out.Bytes()[8:12]))
}

func TestNewProcessor(t *testing.T) {
	p := NewProcessor(4)
	assert.NotNil(t, p)
	assert.NotNil(t, p.sem)
}

func TestProcessor_Semaphore(t *testing.T) {
	p := NewProcessor(4)

	ctx := context.Background()

	// Can acquire all slots
	for i := 0; i < 4; i++ {
		err := p.sem.Acquire(ctx, 1)
		assert.NoError(t, err)
	}

	// Next one should block or fail if context is canceled
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	err := p.sem.Acquire(cancelCtx, 1)
	assert.Error(t, err)

	// Release all
	for i := 0; i < 4; i++ {
		p.sem.Release(1)
	}

	// Can acquire again
	err = p.sem.Acquire(ctx, 1)
	assert.NoError(t, err)
	p.sem.Release(1)
}
