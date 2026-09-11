package inmem

import (
	"context"
	"sync"
	"testing"
	"time"

	appimage "main/internal/application/image"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// drain 在后台 goroutine 中按 minPriority 拉取 n 个任务，返回通道供测试断言
func drain(t *testing.T, q appimage.TranscodeQueue, minPriority appimage.Prio, n int) <-chan appimage.TranscodeJob {
	t.Helper()
	jobs := make(chan appimage.TranscodeJob, n)
	go func() {
		for job, err := range q.Consume(context.Background(), minPriority) {
			if err != nil {
				close(jobs)
				return
			}
			jobs <- job
			if len(jobs) == cap(jobs) {
				return
			}
		}
	}()
	// 等待消费循环注册，避免 Enqueue 先于 Consume 导致的不确定时序
	time.Sleep(10 * time.Millisecond)
	return jobs
}

func TestTranscodeQueue_EnqueueWaitResolve(t *testing.T) {
	q := NewTranscodeQueue()
	item, err := appimage.NewTranscodeItem("k1", "src/a.png", appimage.Spec{}, appimage.PrioLow)
	require.NoError(t, err)

	jobs := drain(t, q, appimage.PrioLow, 1)

	waiter, err := q.Enqueue(context.Background(), item)
	require.NoError(t, err)

	select {
	case job := <-jobs:
		assert.Equal(t, "k1", job.Item().Key())
		go job.Resolve(nil, nil)
	case <-time.After(time.Second):
		t.Fatal("job not yielded")
	}

	file, err := waiter.Wait(context.Background())
	assert.NoError(t, err)
	assert.Nil(t, file)
}

func TestTranscodeQueue_SameKeySharesExecution(t *testing.T) {
	q := NewTranscodeQueue()
	item, err := appimage.NewTranscodeItem("k1", "src/a.png", appimage.Spec{}, appimage.PrioLow)
	require.NoError(t, err)

	jobs := drain(t, q, appimage.PrioLow, 1)

	w1, err := q.Enqueue(context.Background(), item)
	require.NoError(t, err)
	w2, err := q.Enqueue(context.Background(), item)
	require.NoError(t, err)

	select {
	case job := <-jobs:
		go job.Resolve(nil, nil)
	case <-time.After(time.Second):
		t.Fatal("job not yielded")
	}

	// 两个等待者拿到同一结果，且只 yield 一个 job
	_, err1 := w1.Wait(context.Background())
	_, err2 := w2.Wait(context.Background())
	assert.NoError(t, err1)
	assert.NoError(t, err2)
	select {
	case job := <-jobs:
		t.Fatalf("unexpected extra job: %v", job.Item().Key())
	case <-time.After(50 * time.Millisecond):
	}
}

func TestTranscodeQueue_AllWaitersCancelledNotYielded(t *testing.T) {
	q := NewTranscodeQueue()
	item, err := appimage.NewTranscodeItem("k1", "src/a.png", appimage.Spec{}, appimage.PrioLow)
	require.NoError(t, err)

	jobs := drain(t, q, appimage.PrioLow, 1)

	waiter, err := q.Enqueue(context.Background(), item)
	require.NoError(t, err)
	waiter.Cancel()

	select {
	case job := <-jobs:
		t.Fatalf("cancelled item should not be yielded: %v", job.Item().Key())
	case <-time.After(100 * time.Millisecond):
	}
}

func TestTranscodeQueue_PriorityPromotion(t *testing.T) {
	q := NewTranscodeQueue()

	jobs := drain(t, q, appimage.PrioLow, 2)

	// 排队一个低优先级任务，之后同 key 的高优先级等待者到达
	lowItem, err := appimage.NewTranscodeItem("k1", "src/a.png", appimage.Spec{}, appimage.PrioLow)
	require.NoError(t, err)
	lowWaiter, err := q.Enqueue(context.Background(), lowItem)
	require.NoError(t, err)

	highItem, err := appimage.NewTranscodeItem("k1", "src/a.png", appimage.Spec{}, appimage.PrioHigh)
	require.NoError(t, err)
	highWaiter, err := q.Enqueue(context.Background(), highItem)
	require.NoError(t, err)

	// 高优先级等待者撤回后，该 key 仍因低优先级等待者存在而应执行
	highWaiter.Cancel()

	select {
	case job := <-jobs:
		go job.Resolve(nil, nil)
	case <-time.After(time.Second):
		t.Fatal("job not yielded")
	}
	_, err = lowWaiter.Wait(context.Background())
	assert.NoError(t, err)
}

func TestTranscodeQueue_ConsumePriorityThreshold(t *testing.T) {
	q := NewTranscodeQueue()

	// 专职 worker 只收 High
	highJobs := drain(t, q, appimage.PrioHigh, 1)

	lowItem, err := appimage.NewTranscodeItem("low", "src/low.png", appimage.Spec{}, appimage.PrioLow)
	require.NoError(t, err)
	_, err = q.Enqueue(context.Background(), lowItem)
	require.NoError(t, err)

	select {
	case job := <-highJobs:
		t.Fatalf("high-only consumer should not receive low item: %v", job.Item().Key())
	case <-time.After(100 * time.Millisecond):
	}

	// 通用 worker 收 Low
	allJobs := drain(t, q, appimage.PrioLow, 1)
	select {
	case job := <-allJobs:
		go job.Resolve(nil, nil)
	case <-time.After(time.Second):
		t.Fatal("job not yielded")
	}
}

func TestTranscodeQueue_ConsumeAfterCancelFreesSlot(t *testing.T) {
	// 撤回后队列变空，Consume 循环应保持存活可继续收新任务
	q := NewTranscodeQueue()

	jobs := drain(t, q, appimage.PrioLow, 1)

	cancelled, err := appimage.NewTranscodeItem("gone", "src/gone.png", appimage.Spec{}, appimage.PrioLow)
	require.NoError(t, err)
	w, err := q.Enqueue(context.Background(), cancelled)
	require.NoError(t, err)
	w.Cancel()

	next, err := appimage.NewTranscodeItem("next", "src/next.png", appimage.Spec{}, appimage.PrioLow)
	require.NoError(t, err)
	waiter, err := q.Enqueue(context.Background(), next)
	require.NoError(t, err)

	select {
	case job := <-jobs:
		go job.Resolve(nil, nil)
	case <-time.After(time.Second):
		t.Fatal("job not yielded after earlier cancellation")
	}
	_, err = waiter.Wait(context.Background())
	assert.NoError(t, err)
}

func TestTranscodeQueue_ConcurrentEnqueue(t *testing.T) {
	q := NewTranscodeQueue()
	jobs := drain(t, q, appimage.PrioLow, 1)

	const goroutines = 8
	waiters := make([]appimage.TranscodeWaiter, goroutines)
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			item, err := appimage.NewTranscodeItem("shared", "src/shared.png", appimage.Spec{}, appimage.PrioLow)
			require.NoError(t, err)
			w, err := q.Enqueue(context.Background(), item)
			require.NoError(t, err)
			waiters[i] = w
		}(i)
	}
	wg.Wait()

	select {
	case job := <-jobs:
		go job.Resolve(nil, nil)
	case <-time.After(time.Second):
		t.Fatal("job not yielded")
	}

	for _, w := range waiters {
		_, err := w.Wait(context.Background())
		assert.NoError(t, err)
	}
}
