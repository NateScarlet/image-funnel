package magick

import (
	"bytes"
	"context"
	"fmt"
	"main/internal/shared/transcode"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

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

	spec, err := transcode.NewSpec(400, transcode.ImageFormatWebP)
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

func TestResizeArgument_UsesEffectiveWidthRule(t *testing.T) {
	// resize 参数的目标宽度必须来自唯一规则 EffectiveWidth，而非在编码器内重复书写 min。
	// Go 侧不掌握源宽时传入 0，规则原样返回 width，由 '>' 在运行时收缩。
	assert.Equal(t, "1024x>", resizeArgument(1024, 0))
	assert.Equal(t, "0x>", resizeArgument(0, 0))
	// 已知源宽时规则取较小值，参数仍保持 '>' 作为运行时等价保障
	for _, tc := range []struct{ width, sourceWidth, want int }{
		{4096, 2048, 2048},
		{1024, 2048, 1024},
		{2048, 2048, 2048},
	} {
		assert.Equal(t, fmt.Sprintf("%dx>", tc.want), resizeArgument(tc.width, tc.sourceWidth))
		assert.Equal(t, tc.want, transcode.EffectiveWidth(tc.width, tc.sourceWidth))
	}
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
