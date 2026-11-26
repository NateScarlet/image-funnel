package sentry

import (
	"context"
	"fmt"

	domsentry "main/internal/domain/sentry"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ReportingCore 在普通业务 logger 上挂载上报钩子：Error 级别的 error 字段
// 若属于需要开发者处理的错误，就通过 Reporter 上报。
// 接收端与上报链路必须使用不带本 core 的安全 logger，避免错误自激循环。
type ReportingCore struct {
	zapcore.Core
	reporter   domsentry.Reporter
	safeLogger *zap.Logger
	withFields []zapcore.Field
}

var _ zapcore.Core = (*ReportingCore)(nil)

func NewReportingCore(core zapcore.Core, reporter domsentry.Reporter, safeLogger *zap.Logger) zapcore.Core {
	return &ReportingCore{Core: core, reporter: reporter, safeLogger: safeLogger}
}

// Check 让内嵌 core 按原逻辑完成采样与写入决策，再把本 core 加入 CheckedEntry。
// 本 core 的 Write 只负责上报，日志本身由内嵌 core 写入，避免重复输出。
func (c *ReportingCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if !c.Core.Enabled(ent.Level) {
		return ce
	}
	checked := c.Core.Check(ent, ce)
	if checked == nil {
		return nil
	}
	return checked.AddCore(ent, c)
}

func (c *ReportingCore) With(fields []zapcore.Field) zapcore.Core {
	next := &ReportingCore{
		Core:       c.Core.With(fields),
		reporter:   c.reporter,
		safeLogger: c.safeLogger,
		withFields: append(append([]zapcore.Field(nil), c.withFields...), fields...),
	}
	return next
}

func (c *ReportingCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	if entry.Level < zapcore.ErrorLevel {
		return nil
	}

	all := append(append([]zapcore.Field(nil), c.withFields...), fields...)
	reportErr := errorFromFields(all)
	if reportErr == nil || !domsentry.ShouldReport(reportErr) {
		return nil
	}
	if reportErr := c.reporter.Report(context.Background(), domsentry.Event{
		Err:         reportErr,
		Fingerprint: []string{"server", fmt.Sprintf("%T", reportErr)},
	}); reportErr != nil {
		// 上报失败只能进不带钩子的 logger，绝不回灌为新的上报
		c.safeLogger.Error("failed to report server error", zap.Error(reportErr))
	}
	return nil
}

// errorFromFields 只识别常规的 error/err 字段，避免把 panic 等其它字段重复上报。
func errorFromFields(fields []zapcore.Field) error {
	for _, field := range fields {
		if field.Key != "error" && field.Key != "err" {
			continue
		}
		if err, ok := field.Interface.(error); ok {
			return err
		}
	}
	return nil
}
