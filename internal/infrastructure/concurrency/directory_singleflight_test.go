package concurrency

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"main/internal/domain/directory"

	"github.com/stretchr/testify/assert"
)

type mockAnalyzer struct {
	callCount int32
	delay     time.Duration
}

func (m *mockAnalyzer) Analyze(ctx context.Context, relPath string) (*directory.Stats, error) {
	atomic.AddInt32(&m.callCount, 1)
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return directory.NewStats(10, 2, nil, map[int]int{5: 5}), nil
}

func TestSingleFlightDirectoryAnalyzer_MergeRequests(t *testing.T) {
	mock := &mockAnalyzer{
		delay: 50 * time.Millisecond,
	}
	sf := NewSingleFlightDirectoryAnalyzer(mock)

	var wg sync.WaitGroup
	var stats1, stats2 *directory.Stats
	var err1, err2 error

	wg.Add(2)
	go func() {
		defer wg.Done()
		stats1, err1 = sf.Analyze(context.Background(), "test-dir")
	}()
	go func() {
		defer wg.Done()
		// 稍微等待，确保第一个请求已经执行但尚未完成
		time.Sleep(10 * time.Millisecond)
		stats2, err2 = sf.Analyze(context.Background(), "test-dir")
	}()

	wg.Wait()

	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.NotNil(t, stats1)
	assert.NotNil(t, stats2)
	assert.Equal(t, 10, stats1.ImageCount())
	assert.Equal(t, 10, stats2.ImageCount())
	// 验证底层 Analyzer 仅被调用了 1 次
	assert.Equal(t, int32(1), atomic.LoadInt32(&mock.callCount))
}

func TestSingleFlightDirectoryAnalyzer_DifferentPaths(t *testing.T) {
	mock := &mockAnalyzer{
		delay: 50 * time.Millisecond,
	}
	sf := NewSingleFlightDirectoryAnalyzer(mock)

	var wg sync.WaitGroup
	var stats1, stats2 *directory.Stats
	var err1, err2 error

	wg.Add(2)
	go func() {
		defer wg.Done()
		stats1, err1 = sf.Analyze(context.Background(), "dir-a")
	}()
	go func() {
		defer wg.Done()
		time.Sleep(10 * time.Millisecond)
		stats2, err2 = sf.Analyze(context.Background(), "dir-b")
	}()

	wg.Wait()

	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.NotNil(t, stats1)
	assert.NotNil(t, stats2)
	// 验证不同路径的请求不会被合并，底层 Analyzer 被调用了 2 次
	assert.Equal(t, int32(2), atomic.LoadInt32(&mock.callCount))
}
