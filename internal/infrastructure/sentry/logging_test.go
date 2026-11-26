package sentry

import (
	"context"
	"errors"
	"testing"

	"main/internal/apperror"
	domsentry "main/internal/domain/sentry"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type fakeReporter struct {
	events []domsentry.Event
	err    error
}

func (f *fakeReporter) Report(_ context.Context, event domsentry.Event) error {
	f.events = append(f.events, event)
	return f.err
}

func TestReportingCoreReportsUnexpectedErrorLogs(t *testing.T) {
	core, _ := observer.New(zap.DebugLevel)
	reporter := &fakeReporter{}
	logger := zap.New(NewReportingCore(core, reporter, zap.NewNop()))

	logger.Error("failed to save image",
		zap.Error(errors.New("disk full")),
		zap.String("path", `C:\Users\alice\photo.jpg`),
	)

	require.Len(t, reporter.events, 1)
	assert.Equal(t, "disk full", reporter.events[0].Err.Error())
}

func TestReportingCoreSkipsKnownAndRecoverableErrors(t *testing.T) {
	core, _ := observer.New(zap.DebugLevel)
	reporter := &fakeReporter{}
	logger := zap.New(NewReportingCore(core, reporter, zap.NewNop()))

	logger.Error("bad request", zap.Error(apperror.New("BAD_REQUEST", "bad", "错误")))
	logger.Error("canceled", zap.Error(context.Canceled))
	logger.Info("not an error", zap.Error(errors.New("boom")))

	assert.Empty(t, reporter.events, "已知用户错误、可恢复错误和非 Error 日志不得上报")
}

func TestReportingCoreLogsReportFailureWithoutRecursion(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	hookedCore, _ := observer.New(zap.ErrorLevel)
	reporter := &fakeReporter{err: errors.New("network down")}
	logger := zap.New(NewReportingCore(hookedCore, reporter, zap.New(core)))

	logger.Error("boom", zap.Error(errors.New("unexpected")))

	require.Equal(t, 1, logs.Len())
	assert.Contains(t, logs.All()[0].Message, "failed to report")
}
