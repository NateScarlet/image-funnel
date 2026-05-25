package image

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"main/internal/shared"

	"go.uber.org/zap"
)

type mockProcessor struct {
	calls      int
	errs       []error
	resultFile File
	resultMeta *shared.ImageMeta
}

func (m *mockProcessor) Process(ctx context.Context, srcPath string, width, quality int) (File, error) {
	m.calls++
	if m.calls <= len(m.errs) {
		return nil, m.errs[m.calls-1]
	}
	return m.resultFile, nil
}

func (m *mockProcessor) Meta(ctx context.Context, srcPath string) (*shared.ImageMeta, error) {
	m.calls++
	if m.calls <= len(m.errs) {
		return nil, m.errs[m.calls-1]
	}
	return m.resultMeta, nil
}

func TestRetryProcessor_Process_SuccessAfterRetry(t *testing.T) {
	mock := &mockProcessor{
		errs: []error{io.ErrUnexpectedEOF, io.ErrUnexpectedEOF},
	}
	p := NewRetryProcessor(mock, zap.NewNop())
	p.backoff = 1 * time.Millisecond // 缩短退避以加速测试

	_, err := p.Process(context.Background(), "test.png", 100, 75)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	if mock.calls != 3 {
		t.Errorf("expected 3 calls, got %d", mock.calls)
	}
}

func TestRetryProcessor_Process_NonTargetErrorNoRetry(t *testing.T) {
	fatalErr := errors.New("fatal decode error")
	mock := &mockProcessor{
		errs: []error{fatalErr},
	}
	p := NewRetryProcessor(mock, zap.NewNop())
	p.backoff = 1 * time.Millisecond

	_, err := p.Process(context.Background(), "test.png", 100, 75)
	if !errors.Is(err, fatalErr) {
		t.Fatalf("expected fatalErr, got: %v", err)
	}

	if mock.calls != 1 {
		t.Errorf("expected 1 call, got %d", mock.calls)
	}
}

func TestRetryProcessor_Process_ContextCancel(t *testing.T) {
	mock := &mockProcessor{
		errs: []error{io.ErrUnexpectedEOF, io.ErrUnexpectedEOF},
	}
	p := NewRetryProcessor(mock, zap.NewNop())
	p.backoff = 50 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	// 异步取消 context
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_, err := p.Process(ctx, "test.png", 100, 75)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}

	if mock.calls > 2 {
		t.Errorf("expected at most 2 calls before cancellation, got %d", mock.calls)
	}
}

func TestRetryProcessor_Meta_SuccessAfterRetry(t *testing.T) {
	expectedMeta := &shared.ImageMeta{Width: 800, Height: 600}
	mock := &mockProcessor{
		errs:       []error{io.ErrUnexpectedEOF},
		resultMeta: expectedMeta,
	}
	p := NewRetryProcessor(mock, zap.NewNop())
	p.backoff = 1 * time.Millisecond

	meta, err := p.Meta(context.Background(), "test.png")
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	if mock.calls != 2 {
		t.Errorf("expected 2 calls, got %d", mock.calls)
	}

	if meta.Width != 800 || meta.Height != 600 {
		t.Errorf("meta size mismatch")
	}
}
