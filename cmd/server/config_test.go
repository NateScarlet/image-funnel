package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// 说明：loadConfig 依赖 os.Getenv，测试通过 t.Setenv 注入环境变量。
// 未显式设置的变量使用 t.Setenv 置空，避免宿主机环境影响断言结果

func TestLoadConfig_ImageProcessorDefaultsToAuto(t *testing.T) {
	t.Setenv("IMAGE_FUNNEL_IMAGE_PROCESSOR", "")

	cfg, err := loadConfig(zap.NewNop(), "test")
	require.NoError(t, err)
	assert.Equal(t, ImageProcessorAuto, cfg.ImageProcessor,
		"未配置时应为 auto：有 ffmpeg 用 ffmpeg，否则回退 ImageMagick")
}

func TestLoadConfig_ImageProcessorMagick(t *testing.T) {
	t.Setenv("IMAGE_FUNNEL_IMAGE_PROCESSOR", "magick")

	cfg, err := loadConfig(zap.NewNop(), "test")
	require.NoError(t, err)
	assert.Equal(t, ImageProcessorMagick, cfg.ImageProcessor)
}

func TestLoadConfig_ImageProcessorAuto(t *testing.T) {
	t.Setenv("IMAGE_FUNNEL_IMAGE_PROCESSOR", "auto")

	cfg, err := loadConfig(zap.NewNop(), "test")
	require.NoError(t, err)
	assert.Equal(t, ImageProcessorAuto, cfg.ImageProcessor)
}

func TestLoadConfig_ImageProcessorInvalidFailsFast(t *testing.T) {
	// 显式写错的值不能被静默忽略：否则用户以为已生效，实际仍走 ffmpeg
	t.Setenv("IMAGE_FUNNEL_IMAGE_PROCESSOR", "ImageMagick")

	_, err := loadConfig(zap.NewNop(), "test")
	require.Error(t, err, "invalid value must fail fast instead of silently falling back")
	assert.Contains(t, err.Error(), "IMAGE_FUNNEL_IMAGE_PROCESSOR")
}
