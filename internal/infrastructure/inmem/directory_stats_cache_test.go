package inmem_test

import (
	"context"
	"iter"
	"main/internal/domain/directory"
	"main/internal/domain/image"
	"main/internal/infrastructure/inmem"
	"testing"

	"go.uber.org/zap/zaptest"
)

type mockScanner struct {
	analyzeCallCount int
}

func (m *mockScanner) Scan(ctx context.Context, relPath string) iter.Seq2[*image.Image, error] {
	return nil
}
func (m *mockScanner) LookupImage(ctx context.Context, relPath string) (*image.Image, error) {
	return nil, nil
}
func (m *mockScanner) ScanDirectories(ctx context.Context, relPath string) iter.Seq2[*directory.Directory, error] {
	return nil
}
func (m *mockScanner) AnalyzeDirectory(ctx context.Context, relPath string) (*directory.DirectoryStats, error) {
	m.analyzeCallCount++
	return directory.NewDirectoryStats(10, 5, nil, map[int]int{}), nil
}

func TestDirectoryStatsCache(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mock := &mockScanner{}

	cache := inmem.NewDirectoryStatsCache(mock, logger)
	ctx := context.Background()

	// 第一次调用应该穿透到 mock
	stats1, err := cache.AnalyzeDirectory(ctx, "test/dir")
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
	_, err = cache.AnalyzeDirectory(ctx, "test/dir")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.analyzeCallCount != 1 {
		t.Fatalf("expected 1 call, got %d", mock.analyzeCallCount)
	}

	// 手动使缓存作废
	cache.Invalidate("test/dir")

	// 第三次调用应该重新穿透到 mock
	_, err = cache.AnalyzeDirectory(ctx, "test/dir")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.analyzeCallCount != 2 {
		t.Fatalf("expected 2 calls, got %d", mock.analyzeCallCount)
	}
}
