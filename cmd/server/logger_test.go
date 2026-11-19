package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestParseDebugLoggerNames(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  map[string]struct{}
	}{
		{"empty", "", map[string]struct{}{}},
		{"single", "commit-pipeline", map[string]struct{}{"commit-pipeline": {}}},
		{"multiple with spaces", " a ,b, a ", map[string]struct{}{"a": {}, "b": {}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseDebugLoggerNames(tt.value))
		})
	}
}

// captureStderr 捕获 initLogger 写入 stderr 的日志，避免依赖文件 sink 的生命周期。
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	oldStderr := os.Stderr
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = writer
	defer func() { os.Stderr = oldStderr }()

	fn()
	require.NoError(t, writer.Close())
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	return string(data)
}

func TestInitLogger_DebugOnlyAfterOptInByName(t *testing.T) {
	// 未点名：被命名 logger 的 Debug 不输出，Info 正常输出且不重复
	content := captureStderr(t, func() {
		t.Setenv(debugLoggersEnv, "")
		logger, err := initLogger("release")
		require.NoError(t, err)
		logger.Named(commitPipelineLoggerName).Debug("sampling timing")
		logger.Info("normal info")
	})
	assert.NotContains(t, content, "sampling timing")
	assert.Equal(t, 1, strings.Count(content, "normal info"))

	// 点名后：只有该名字的 Debug 额外输出，其它 logger 的 Debug 与 Info 都不重复
	content = captureStderr(t, func() {
		t.Setenv(debugLoggersEnv, commitPipelineLoggerName)
		logger, err := initLogger("release")
		require.NoError(t, err)
		logger.Named(commitPipelineLoggerName).Debug("sampling timing", zap.String("phase", "generate"))
		logger.Named("other").Debug("other debug")
		logger.Info("normal info")
	})
	assert.Contains(t, content, "sampling timing")
	assert.Contains(t, content, "generate")
	assert.NotContains(t, content, "other debug")
	assert.Equal(t, 1, strings.Count(content, "normal info"))
}

func TestLevelOverrideCore_RoutesOnlyNamedDebugLogs(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	debugCore, debugLogs := observer.New(zap.DebugLevel)
	override := levelOverrideCore{
		Core:       core,
		debug:      debugCore,
		debugNames: map[string]struct{}{"target": {}},
	}
	logger := zap.New(override)

	named := logger.Named("target")
	other := logger.Named("other")

	// 被点名 logger 的 Debug 只走 debug core；普通 core 不应收到
	assert.True(t, named.Core().Enabled(zapcore.DebugLevel))
	named.Debug("target debug")
	named.Info("target info")
	other.Debug("other debug")
	other.Info("other info")
	logger.Debug("root debug")
	logger.Info("root info")

	assert.Equal(t, 0, logs.FilterMessage("target debug").Len())
	assert.Equal(t, 0, logs.FilterMessage("other debug").Len())
	assert.Equal(t, 0, logs.FilterMessage("root debug").Len())
	assert.Equal(t, 1, logs.FilterMessage("target info").Len())
	assert.Equal(t, 1, logs.FilterMessage("other info").Len())
	assert.Equal(t, 1, logs.FilterMessage("root info").Len())

	assert.Equal(t, 1, debugLogs.FilterMessage("target debug").Len())
	assert.Equal(t, 0, debugLogs.FilterMessage("other debug").Len())
	assert.Equal(t, 0, debugLogs.FilterMessage("root debug").Len())
	// 点名 logger 的 Info 不属于 Debug 路由，避免重复输出
	assert.Equal(t, 0, debugLogs.FilterMessage("target info").Len())

	// 总条数验证：Info 只出现一次，Debug 只出现一次
	assert.Equal(t, 3, logs.Len())
	assert.Equal(t, 1, debugLogs.Len())
}

func TestLevelOverrideCore_EmptyNamesKeepsDefaultBehavior(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	debugCore, debugLogs := observer.New(zap.DebugLevel)
	logger := zap.New(levelOverrideCore{
		Core:       core,
		debug:      debugCore,
		debugNames: map[string]struct{}{},
	})

	logger.Debug("hidden")
	logger.Info("shown")
	assert.False(t, logger.Core().Enabled(zapcore.DebugLevel))
	assert.Equal(t, 0, logs.FilterMessage("hidden").Len())
	assert.Equal(t, 1, logs.FilterMessage("shown").Len())
	assert.Equal(t, 0, debugLogs.Len())
}

func TestLevelOverrideCore_WithFieldsPreservesRouting(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	debugCore, debugLogs := observer.New(zap.DebugLevel)
	logger := zap.New(levelOverrideCore{
		Core:       core,
		debug:      debugCore,
		debugNames: map[string]struct{}{"target": {}},
	}, zap.Fields(zap.String("base", "field")))

	logger.Named("target").Debug("target debug", zap.Int("extra", 1))
	assert.Equal(t, 0, logs.Len())
	entries := debugLogs.All()
	require.Len(t, entries, 1)
	assert.Equal(t, "target", entries[0].LoggerName)
	assert.Equal(t, "field", entries[0].ContextMap()["base"])
	assert.Equal(t, int64(1), entries[0].ContextMap()["extra"])
}
