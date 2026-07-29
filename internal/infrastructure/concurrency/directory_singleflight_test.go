package concurrency

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"main/internal/domain/directory"

	"github.com/stretchr/testify/assert"
)

// #region Helper Structs

type mockAnalyzer struct {
	callCount int32
	started   chan struct{}
	release   chan struct{}
}

func (m *mockAnalyzer) Analyze(ctx context.Context, relPath string) (*directory.Stats, error) {
	atomic.AddInt32(&m.callCount, 1)

	// 通知调用方，Analyze 已经接收到请求并进入处理流程
	if m.started != nil {
		select {
		case m.started <- struct{}{}:
		default:
		}
	}

	// 阻塞在此处直到 release 信号被触发，防止底层 Analyze 过早完成导致 singleflight 缓存被清空
	if m.release != nil {
		select {
		case <-m.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return directory.NewStats(10, 2, nil, map[int]int{5: 5}), nil
}

// #endregion

// #region Tests

func TestSingleFlightDirectoryAnalyzer_MergeRequests(t *testing.T) {
	mockStarted := make(chan struct{}, 1)
	release := make(chan struct{})
	mock := &mockAnalyzer{
		started: mockStarted,
		release: release,
	}
	sf := NewSingleFlightDirectoryAnalyzer(mock)

	var wg sync.WaitGroup
	var stats1, stats2 *directory.Stats
	var err1, err2 error

	enter2 := make(chan struct{}, 1)

	wg.Add(2)

	// 发起第一个请求
	go func() {
		defer wg.Done()
		stats1, err1 = sf.Analyze(context.Background(), "test-dir")
	}()

	// 明确等待第一个请求已被底层 mock.Analyze 接收并挂起
	<-mockStarted

	// 发起第二个请求，并在调用 sf.Analyze 前发出信号
	go func() {
		defer wg.Done()
		enter2 <- struct{}{}
		stats2, err2 = sf.Analyze(context.Background(), "test-dir")
	}()

	// 等待第二个 goroutine 启动并进入 Analyze 方法过程
	<-enter2

	// 让出 CPU 切片，确保第二个 goroutine 进入 singleflight Do 方法内部挂起
	for i := 0; i < 5; i++ {
		runtime.Gosched()
		time.Sleep(1 * time.Millisecond)
	}

	// 此时第一个请求仍然被 release 阻塞，第二个请求一定排在 singleflight 队列中。
	// 释放第一个请求，允许 singleflight 完成并分发给两个调用者
	close(release)

	wg.Wait()

	// 验证两个请求均成功获取到了统计数据
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
	mock := &mockAnalyzer{}
	sf := NewSingleFlightDirectoryAnalyzer(mock)

	var wg sync.WaitGroup
	var stats1, stats2 *directory.Stats
	var err1, err2 error

	wg.Add(2)

	// 发起针对 dir-a 的请求
	go func() {
		defer wg.Done()
		stats1, err1 = sf.Analyze(context.Background(), "dir-a")
	}()

	// 发起针对 dir-b 的请求
	go func() {
		defer wg.Done()
		stats2, err2 = sf.Analyze(context.Background(), "dir-b")
	}()

	wg.Wait()

	// 验证两个请求均成功获取到了统计数据
	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.NotNil(t, stats1)
	assert.NotNil(t, stats2)

	// 验证不同路径的请求不会被合并，底层 Analyzer 被调用了 2 次
	assert.Equal(t, int32(2), atomic.LoadInt32(&mock.callCount))
}

// #endregion
