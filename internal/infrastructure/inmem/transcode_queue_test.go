package inmem

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"main/internal/shared/transcode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// drain 在后台 goroutine 中按 minPriority 拉取 n 个任务，返回通道供测试断言
func drain(t *testing.T, q transcode.Queue, minPriority transcode.Prio, n int) <-chan transcode.Job {
	t.Helper()
	jobs := make(chan transcode.Job, n)
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
	item, err := transcode.NewItem("k1", "src/a.png", transcode.Spec{}, transcode.PrioLow, nil)
	require.NoError(t, err)

	jobs := drain(t, q, transcode.PrioLow, 1)

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
	item, err := transcode.NewItem("k1", "src/a.png", transcode.Spec{}, transcode.PrioLow, nil)
	require.NoError(t, err)

	jobs := drain(t, q, transcode.PrioLow, 1)

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

func TestTranscodeQueue_AllWaitersWithdrawnNotYielded(t *testing.T) {
	// 契约只保证「等待者已全部撤回的未启动任务不会产出」（见 Queue）：
	// 消费循环一旦先行启动就可能抢在撤回之前认领该任务，而认领后
	// 再撤回已无法召回。因此先撤回需求再启动消费，才能确定性地验证淘汰逻辑
	q := NewTranscodeQueue()
	ctx, cancel := context.WithCancel(context.Background())
	item, err := transcode.NewItem("k1", "src/a.png", transcode.Spec{}, transcode.PrioLow, nil)
	require.NoError(t, err)

	_, err = q.Enqueue(ctx, item)
	require.NoError(t, err)
	// Enqueue 的 ctx 结束即撤回该等待者的登记
	cancel()

	jobs := drain(t, q, transcode.PrioLow, 1)

	select {
	case job := <-jobs:
		t.Fatalf("withdrawn item should not be yielded: %v", job.Item().Key())
	case <-time.After(100 * time.Millisecond):
	}
}

func TestTranscodeQueue_WithdrawalWhileConsumerBlocked(t *testing.T) {
	// 覆盖常驻消费循环已阻塞在 notify 上的情形：等待者此时 abort，
	// 需求应在下一次扫描时被淘汰，而不是被误认领
	q := NewTranscodeQueue()
	ctx, stopConsume := context.WithCancel(context.Background())
	defer stopConsume()

	lowItem, err := transcode.NewItem("low", "src/low.png", transcode.Spec{}, transcode.PrioLow, nil)
	require.NoError(t, err)
	lowCtx, withdraw := context.WithCancel(context.Background())
	_, err = q.Enqueue(lowCtx, lowItem)
	require.NoError(t, err)

	// 只收 High 的 worker 扫不到候选，阻塞在 notify 上
	jobs := make(chan transcode.Job, 1)
	go func() {
		for job, err := range q.Consume(ctx, transcode.PrioHigh) {
			if err != nil {
				return
			}
			jobs <- job
			return
		}
	}()
	time.Sleep(20 * time.Millisecond)
	withdraw()

	// 新需求唤醒消费循环；扫描时应先淘汰已撤回的 low，再认领 high
	highItem, err := transcode.NewItem("high", "src/high.png", transcode.Spec{}, transcode.PrioHigh, nil)
	require.NoError(t, err)
	_, err = q.Enqueue(context.Background(), highItem)
	require.NoError(t, err)

	select {
	case job := <-jobs:
		assert.Equal(t, "high", job.Item().Key())
		job.Resolve(nil, nil)
	case <-time.After(time.Second):
		t.Fatal("job not yielded after withdrawal")
	}
}

func TestTranscodeQueue_PriorityPromotionKeepsLowerWaiter(t *testing.T) {
	q := NewTranscodeQueue()

	jobs := drain(t, q, transcode.PrioLow, 2)

	lowItem, err := transcode.NewItem("k1", "src/a.png", transcode.Spec{}, transcode.PrioLow, nil)
	require.NoError(t, err)
	lowWaiter, err := q.Enqueue(context.Background(), lowItem)
	require.NoError(t, err)

	highCtx, cancelHigh := context.WithCancel(context.Background())
	highItem, err := transcode.NewItem("k1", "src/a.png", transcode.Spec{}, transcode.PrioHigh, nil)
	require.NoError(t, err)
	_, err = q.Enqueue(highCtx, highItem)
	require.NoError(t, err)

	// 高优先级等待者撤回后，该 key 仍因低优先级等待者存在而应执行
	cancelHigh()

	select {
	case job := <-jobs:
		go job.Resolve(nil, nil)
	case <-time.After(time.Second):
		t.Fatal("job not yielded")
	}
	_, err = lowWaiter.Wait(context.Background())
	assert.NoError(t, err)
}

func TestTranscodeQueue_PriorityPromotionAllowsHighWorker(t *testing.T) {
	q := NewTranscodeQueue()
	lowItem, err := transcode.NewItem("k1", "src/a.png", transcode.Spec{}, transcode.PrioLow, nil)
	require.NoError(t, err)
	_, err = q.Enqueue(context.Background(), lowItem)
	require.NoError(t, err)

	// 同 key 高优先级等待者到达 → 任务优先级提升
	highItem, err := transcode.NewItem("k1", "src/a.png", transcode.Spec{}, transcode.PrioHigh, nil)
	require.NoError(t, err)
	waiter, err := q.Enqueue(context.Background(), highItem)
	require.NoError(t, err)

	// 提升后再启动只收 High 的专职 worker，它应认领到 High 任务
	highJobs := drain(t, q, transcode.PrioHigh, 1)
	select {
	case job := <-highJobs:
		assert.Equal(t, transcode.PrioHigh, job.Item().Priority())
		go job.Resolve(nil, nil)
	case <-time.After(time.Second):
		t.Fatal("promoted job not yielded to high-only consumer")
	}
	_, err = waiter.Wait(context.Background())
	assert.NoError(t, err)
}

func TestTranscodeQueue_ConsumePriorityThreshold(t *testing.T) {
	q := NewTranscodeQueue()

	// 专职 worker 只收 High
	highJobs := drain(t, q, transcode.PrioHigh, 1)

	lowItem, err := transcode.NewItem("low", "src/low.png", transcode.Spec{}, transcode.PrioLow, nil)
	require.NoError(t, err)
	_, err = q.Enqueue(context.Background(), lowItem)
	require.NoError(t, err)

	select {
	case job := <-highJobs:
		t.Fatalf("high-only consumer should not receive low item: %v", job.Item().Key())
	case <-time.After(100 * time.Millisecond):
	}

	// 通用 worker 收 Low
	allJobs := drain(t, q, transcode.PrioLow, 1)
	select {
	case job := <-allJobs:
		go job.Resolve(nil, nil)
	case <-time.After(time.Second):
		t.Fatal("job not yielded")
	}
}

func TestTranscodeQueue_ConsumeAfterWithdrawalFreesSlot(t *testing.T) {
	// 撤回后队列变空，Consume 循环应保持存活可继续收新任务
	q := NewTranscodeQueue()

	// 同 AllWaitersWithdrawnNotYielded：撤回先于消费启动，
	// 否则被撤回的任务可能被先认领，占掉本用例唯一一次拉取机会
	withdrawnCtx, withdraw := context.WithCancel(context.Background())
	cancelled, err := transcode.NewItem("gone", "src/gone.png", transcode.Spec{}, transcode.PrioLow, nil)
	require.NoError(t, err)
	_, err = q.Enqueue(withdrawnCtx, cancelled)
	require.NoError(t, err)
	withdraw()

	next, err := transcode.NewItem("next", "src/next.png", transcode.Spec{}, transcode.PrioLow, nil)
	require.NoError(t, err)
	waiter, err := q.Enqueue(context.Background(), next)
	require.NoError(t, err)

	jobs := drain(t, q, transcode.PrioLow, 1)
	select {
	case job := <-jobs:
		if key := job.Item().Key(); key != "next" {
			t.Fatalf("expected the live item to be yielded, got %v", key)
		}
		go job.Resolve(nil, nil)
	case <-time.After(time.Second):
		t.Fatal("job not yielded after earlier withdrawal")
	}
	_, err = waiter.Wait(context.Background())
	assert.NoError(t, err)
}

func TestTranscodeQueue_ClaimedJobRemainsDedupable(t *testing.T) {
	q := NewTranscodeQueue()
	jobs := drain(t, q, transcode.PrioLow, 1)
	item, err := transcode.NewItem("k1", "src/a.png", transcode.Spec{}, transcode.PrioLow, nil)
	require.NoError(t, err)

	first, err := q.Enqueue(context.Background(), item)
	require.NoError(t, err)

	var job transcode.Job
	select {
	case job = <-jobs:
	case <-time.After(time.Second):
		t.Fatal("job not yielded")
	}

	// 任务已被认领但尚未 Resolve：同 key 的新需求仍应搭乘同一执行
	second, err := q.Enqueue(context.Background(), item)
	require.NoError(t, err)
	job.Resolve(nil, nil)

	_, err1 := first.Wait(context.Background())
	_, err2 := second.Wait(context.Background())
	assert.NoError(t, err1)
	assert.NoError(t, err2)
	select {
	case extra := <-jobs:
		t.Fatalf("unexpected extra job: %v", extra.Item().Key())
	case <-time.After(50 * time.Millisecond):
	}
}

func TestTranscodeQueue_WaitContextDoesNotWithdrawDemand(t *testing.T) {
	// Wait 的 ctx 只结束本次等待；撤回由 Enqueue 的 ctx 表达
	q := NewTranscodeQueue()
	item, err := transcode.NewItem("k1", "src/a.png", transcode.Spec{}, transcode.PrioLow, nil)
	require.NoError(t, err)
	waiter, err := q.Enqueue(context.Background(), item)
	require.NoError(t, err)

	waitCtx, cancelWait := context.WithCancel(context.Background())
	cancelWait()
	_, err = waiter.Wait(waitCtx)
	assert.ErrorIs(t, err, context.Canceled)

	jobs := drain(t, q, transcode.PrioLow, 1)
	select {
	case job := <-jobs:
		go job.Resolve(nil, nil)
	case <-time.After(time.Second):
		t.Fatal("demand was withdrawn by Wait ctx")
	}
	_, err = waiter.Wait(context.Background())
	assert.NoError(t, err)
}

func TestTranscodeQueue_ConcurrentEnqueue(t *testing.T) {
	q := NewTranscodeQueue()
	jobs := drain(t, q, transcode.PrioLow, 1)

	const goroutines = 8
	waiters := make([]transcode.Waiter, goroutines)
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			item, err := transcode.NewItem("shared", "src/shared.png", transcode.Spec{}, transcode.PrioLow, nil)
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

// #region 结果句柄契约

type testFile struct{ data []byte }

func (f testFile) Open() (io.ReadSeekCloser, error) {
	return &testReadSeekCloser{Reader: bytes.NewReader(f.data)}, nil
}

type testReadSeekCloser struct{ *bytes.Reader }

func (c *testReadSeekCloser) Close() error { return nil }

func TestTranscodeQueue_FileOpenReturnsIndependentReaders(t *testing.T) {
	// File 句柄必须可被重复打开，且每次 Open 得到独立偏移的读取器；
	// 队列只负责把结果交出，不改变这一语义
	q := NewTranscodeQueue()
	jobs := drain(t, q, transcode.PrioLow, 1)
	item, err := transcode.NewItem("k1", "src/a.png", transcode.Spec{}, transcode.PrioLow, nil)
	require.NoError(t, err)
	waiter, err := q.Enqueue(context.Background(), item)
	require.NoError(t, err)

	select {
	case job := <-jobs:
		job.Resolve(testFile{data: []byte("abcdef")}, nil)
	case <-time.After(time.Second):
		t.Fatal("job not yielded")
	}

	file, err := waiter.Wait(context.Background())
	require.NoError(t, err)
	r1, err := file.Open()
	require.NoError(t, err)
	defer r1.Close()
	r2, err := file.Open()
	require.NoError(t, err)
	defer r2.Close()

	buf1 := make([]byte, 3)
	buf2 := make([]byte, 3)
	_, err = io.ReadFull(r1, buf1)
	require.NoError(t, err)
	_, err = io.ReadFull(r2, buf2)
	require.NoError(t, err)
	assert.Equal(t, "abc", string(buf1))
	assert.Equal(t, "abc", string(buf2))

	_, err = io.ReadFull(r1, buf1)
	require.NoError(t, err)
	assert.Equal(t, "def", string(buf1))

	// r2 仍停在首次读取后的偏移，证明两个读取器互不影响
	_, err = io.ReadFull(r2, buf2)
	require.NoError(t, err)
	assert.Equal(t, "def", string(buf2))
}

// #endregion
