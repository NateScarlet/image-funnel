package loadlatency

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"main/internal/domain/loadlatency"
)

const validRunID = "0123456789abcdef0123456789abcdef"

type fakeAppender struct {
	called bool
	runID  string
	data   []byte
	path   string
	err    error
}

func (f *fakeAppender) Append(_ context.Context, runID string, data []byte) (string, error) {
	f.called = true
	f.runID = runID
	f.data = data
	return f.path, f.err
}

func TestHandlerAppendSegmentValidatesBeforeAppending(t *testing.T) {
	for _, tc := range []struct {
		name   string
		runID  string
		segment string
	}{
		{name: "invalid run id", runID: "uppercase-ABC", segment: "{}\n"},
		{name: "oversized segment", runID: validRunID, segment: strings.Repeat("a", loadlatency.MaxSegmentBytes+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			appender := &fakeAppender{path: "/data/load-latency/ignored.jsonl"}
			h := NewHandler(appender, zap.NewNop())

			_, err := h.AppendSegment(context.Background(), tc.runID, tc.segment)
			require.Error(t, err)
			assert.False(t, appender.called, "校验失败不得触达落盘")
		})
	}
}

func TestHandlerAppendSegmentAppendsAndReturnsAbsolutePath(t *testing.T) {
	const path = "C:/data/load-latency/0123456789abcdef0123456789abcdef.jsonl"
	appender := &fakeAppender{path: path}
	h := NewHandler(appender, zap.NewNop())

	got, err := h.AppendSegment(context.Background(), validRunID, "{\"segmentSeq\":1}\n")
	require.NoError(t, err)
	assert.Equal(t, path, got)
	assert.True(t, appender.called)
	assert.Equal(t, validRunID, appender.runID)
	assert.JSONEq(t, `{"segmentSeq":1}`, strings.TrimSpace(string(appender.data)))
}

func TestHandlerAppendSegmentPropagatesAppenderError(t *testing.T) {
	wantErr := errors.New("disk full")
	appender := &fakeAppender{err: wantErr}
	h := NewHandler(appender, zap.NewNop())

	_, err := h.AppendSegment(context.Background(), validRunID, "{}\n")
	require.ErrorIs(t, err, wantErr, "落盘失败必须向调用者暴露，不能记录日志后假装成功")
}
