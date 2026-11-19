package main

import (
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// debugLoggersEnv 按 logger 名字（逗号分隔）按需把选中 logger 的日志级别降到 Debug。
// 未设置或为空时不改变任何默认行为；设置后只额外显示被点名 logger 的 Debug 日志，
// 其它 logger 的级别与其它日志的级别都不变。
const debugLoggersEnv = "IMAGE_FUNNEL_DEBUG_LOGGERS"

// commitPipelineLoggerName 是提交后采样流水线的 logger 名字，设置
// IMAGE_FUNNEL_DEBUG_LOGGERS=commit-pipeline 即可查看采样与钩子各阶段耗时日志。
const commitPipelineLoggerName = "commit-pipeline"

func initLogger(version string) (*zap.Logger, error) {
	cfg := zap.NewProductionConfig()
	if version == "dev" {
		cfg = zap.NewDevelopmentConfig()
	}

	debugNames := parseDebugLoggerNames(os.Getenv(debugLoggersEnv))
	if len(debugNames) == 0 {
		// 未设置时完全保持原有行为，连额外的输出流都不打开
		return cfg.Build()
	}

	// 被点名 logger 的 Debug 日志走独立的 Debug core；其余一切保持 cfg 的原始配置。
	// 两个 core 复用同一输出流，因此不会产生重复或丢失的日志。
	debugCore, err := newDebugCore(cfg)
	if err != nil {
		return nil, err
	}
	return buildLogger(cfg, debugCore, debugNames)
}

// buildLogger 按原始配置构建 logger，并叠加按名字路由 Debug 的核心包装。
func buildLogger(cfg zap.Config, debugCore zapcore.Core, debugNames map[string]struct{}) (*zap.Logger, error) {
	return cfg.Build(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return levelOverrideCore{
			Core:       core,
			debug:      debugCore,
			debugNames: debugNames,
		}
	}))
}

// newDebugCore 用与 cfg 相同的编码与输出流构建一个 Debug 级 core。
func newDebugCore(cfg zap.Config) (zapcore.Core, error) {
	sink, _, err := zap.Open(cfg.OutputPaths...)
	if err != nil {
		return nil, fmt.Errorf("open debug log output: %w", err)
	}
	var encoder zapcore.Encoder
	if cfg.Encoding == "console" {
		encoder = zapcore.NewConsoleEncoder(cfg.EncoderConfig)
	} else {
		encoder = zapcore.NewJSONEncoder(cfg.EncoderConfig)
	}
	return zapcore.NewCore(encoder, sink, zapcore.DebugLevel), nil
}

func parseDebugLoggerNames(value string) map[string]struct{} {
	names := make(map[string]struct{})
	for _, part := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			names[trimmed] = struct{}{}
		}
	}
	return names
}

// levelOverrideCore 按 logger 名字把 Debug 级条目路由到 debug core，其余交给内嵌的普通 core。
// Logger 名字来自 Entry.LoggerName（Named 方法设置），因此其它功能只需
// logger.Named("自己的名字") 并在环境变量中点亮该名字即可复用本机制。
type levelOverrideCore struct {
	zapcore.Core
	debug      zapcore.Core
	debugNames map[string]struct{}
}

var _ zapcore.Core = levelOverrideCore{}

// Enabled 必须放行 Debug：此时没有名字信息，放行后由 Check 按名字决定是否真正输出。
func (c levelOverrideCore) Enabled(lvl zapcore.Level) bool {
	return c.Core.Enabled(lvl) || (len(c.debugNames) > 0 && lvl == zapcore.DebugLevel)
}

func (c levelOverrideCore) With(fields []zapcore.Field) zapcore.Core {
	return levelOverrideCore{
		Core:       c.Core.With(fields),
		debug:      c.debug.With(fields),
		debugNames: c.debugNames,
	}
}

func (c levelOverrideCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if ent.Level == zapcore.DebugLevel {
		if _, enabled := c.debugNames[ent.LoggerName]; enabled {
			return c.debug.Check(ent, ce)
		}
	}
	return c.Core.Check(ent, ce)
}
