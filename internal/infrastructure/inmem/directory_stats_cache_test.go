package inmem_test

import (
	"context"
	"main/internal/domain/directory"
	"main/internal/infrastructure/inmem"
	"testing"

	"go.uber.org/zap/zaptest"
)

type mockAnalyzer struct {
	analyzeCallCount int
}

func (m *mockAnalyzer) Analyze(ctx context.Context, relPath string) (*directory.Stats, error) {
	m.analyzeCallCount++
	return directory.NewStats(10, 5, nil, map[int]int{}), nil
}

func TestDirectoryStatsCache(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mock := &mockAnalyzer{}

	cache := inmem.NewDirectoryStatsCache(mock, logger)
	ctx := context.Background()

	// 第一次调用应该穿透到 mock
	stats1, err := cache.Analyze(ctx, "test/dir")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats1.ImageCount() != 10 {
		t.Fatalf("expected 10 images, got %d", stats1.ImageCount())
	}
	if mock.analyzeCallCount != 1 {
		t.Fatalf("expected 1 call, got %d", mock.analyzeCallCount)
	}

	// 第二次调用应该命中缓存
	_, err = cache.Analyze(ctx, "test/dir")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.analyzeCallCount != 1 {
		t.Fatalf("expected 1 call, got %d", mock.analyzeCallCount)
	}

	// 手动使缓存作废
	cache.Invalidate("test/dir")

	// 第三次调用应该重新穿透到 mock
	_, err = cache.Analyze(ctx, "test/dir")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.analyzeCallCount != 2 {
		t.Fatalf("expected 2 calls, got %d", mock.analyzeCallCount)
	}
}
