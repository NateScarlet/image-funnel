package main

import (
	"testing"

	"main/internal/domain/sentry"

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

func TestLoadConfig_SentryDisabledByDefault(t *testing.T) {
	t.Setenv("IMAGE_FUNNEL_SENTRY_DSN", "")

	cfg, err := loadConfig(zap.NewNop(), "test")
	require.NoError(t, err)
	assert.Equal(t, sentry.ModeOff, cfg.SentryMode)
	assert.Empty(t, cfg.SentryDSN)
}

func TestLoadConfig_SentrySelfHostedGeneratesEnvelopeDSN(t *testing.T) {
	t.Setenv("IMAGE_FUNNEL_SENTRY_DSN", sentry.SelfDSNValue)
	t.Setenv("IMAGE_FUNNEL_BASE_URL", "http://localhost:34898")

	cfg, err := loadConfig(zap.NewNop(), "test")
	require.NoError(t, err)
	assert.Equal(t, sentry.ModeSelf, cfg.SentryMode)
	assert.Equal(t, "http://self@localhost:34898/0", cfg.SentryDSN)
}

func TestLoadConfig_SentrySelfHostedVariantIsNotSelfMode(t *testing.T) {
	t.Setenv("IMAGE_FUNNEL_SENTRY_DSN", sentry.SelfDSNValue+"extra")

	_, err := loadConfig(zap.NewNop(), "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "IMAGE_FUNNEL_SENTRY_DSN")
}

func TestLoadConfig_SentryExternalDSNPassthrough(t *testing.T) {
	const dsn = "https://abc123@o12345.ingest.sentry.io/67890"
	t.Setenv("IMAGE_FUNNEL_SENTRY_DSN", dsn)

	cfg, err := loadConfig(zap.NewNop(), "test")
	require.NoError(t, err)
	assert.Equal(t, sentry.ModeExternal, cfg.SentryMode)
	assert.Equal(t, dsn, cfg.SentryDSN)
}

func TestLoadConfig_SentryInvalidDSNFailsFast(t *testing.T) {
	t.Setenv("IMAGE_FUNNEL_SENTRY_DSN", "not-a-dsn")

	_, err := loadConfig(zap.NewNop(), "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "IMAGE_FUNNEL_SENTRY_DSN")
}

func TestLoadConfig_TrainingSampleArchiveDisabledByDefault(t *testing.T) {
	// 零值安全：未配置时完全不采集
	t.Setenv("IMAGE_FUNNEL_ENABLE_TRAINING_SAMPLE_ARCHIVE", "")

	cfg, err := loadConfig(zap.NewNop(), "test")
	require.NoError(t, err)
	assert.False(t, cfg.EnableTrainingSampleArchive)
}

func TestLoadConfig_TrainingSampleArchiveExplicitlyEnabled(t *testing.T) {
	t.Setenv("IMAGE_FUNNEL_ENABLE_TRAINING_SAMPLE_ARCHIVE", "true")

	cfg, err := loadConfig(zap.NewNop(), "test")
	require.NoError(t, err)
	assert.True(t, cfg.EnableTrainingSampleArchive)
}

func TestLoadConfig_TrainingSampleArchiveInvalidValueDisabled(t *testing.T) {
	// 无法解析的值不能当作开启，否则会意外开始归档
	t.Setenv("IMAGE_FUNNEL_ENABLE_TRAINING_SAMPLE_ARCHIVE", "yes")

	cfg, err := loadConfig(zap.NewNop(), "test")
	require.NoError(t, err)
	assert.False(t, cfg.EnableTrainingSampleArchive)
}

func TestLoadConfig_LoadLatencyThresholdDisabled(t *testing.T) {
	// 未设置、零值与负值都表示关闭性能上报，零值是安全默认
	for _, tc := range []struct {
		name  string
		value string
	}{
		{name: "unset", value: ""},
		{name: "zero", value: "0"},
		{name: "negative", value: "-5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("IMAGE_FUNNEL_LOAD_LATENCY_THRESHOLD_MS", tc.value)

			cfg, err := loadConfig(zap.NewNop(), "test")
			require.NoError(t, err)
			assert.Zero(t, cfg.LoadLatencyThresholdMs)
		})
	}
}

func TestLoadConfig_LoadLatencyThresholdEnabled(t *testing.T) {
	t.Setenv("IMAGE_FUNNEL_LOAD_LATENCY_THRESHOLD_MS", "750")

	cfg, err := loadConfig(zap.NewNop(), "test")
	require.NoError(t, err)
	assert.Equal(t, 750, cfg.LoadLatencyThresholdMs)
}

func TestLoadConfig_LoadLatencyThresholdInvalidValueDisabled(t *testing.T) {
	// 无法解析的值不能当作开启，否则会意外开启性能上报
	t.Setenv("IMAGE_FUNNEL_LOAD_LATENCY_THRESHOLD_MS", "fast")

	cfg, err := loadConfig(zap.NewNop(), "test")
	require.NoError(t, err)
	assert.Zero(t, cfg.LoadLatencyThresholdMs)
}

