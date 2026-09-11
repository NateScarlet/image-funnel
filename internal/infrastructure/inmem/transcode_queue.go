package inmem

import (
	"context"
	"iter"
	"sync"

	appimage "main/internal/application/image"
)

// transcodeEntry 一个去重后的转码任务：需求记录 + 共享结果 + 等待者计数。
// 从被 Enqueue 创建到 Resolve 为止始终保留在队列映射中——即使已被 Consume
// 取走执行——确保运行中的 key 也能去重（后到等待者搭同一趟执行）
type transcodeEntry struct {
	item  appimage.TranscodeItem
	queue *TranscodeQueue
	mu    sync.Mutex
	// waiters 等待者计数。受队列锁与 e.mu 双重保护：登记/撤回由队列锁串行化，
	// e.mu 保证与结果字段的原子读写
	waiters  int
	claimed  bool
	resolved bool
	file     appimage.File
	err      error
	done     chan struct{}
}

// addWaiter 登记一个等待者。已解决的任务不再接受登记（本 key 需要重新入队）。
// 调用方必须在持有队列锁时调用：映射中存在未解决任务与等待者计数增减的交错
// 由队列锁串行化，entry 自身的 mu 只保护结果字段的读写
func (e *transcodeEntry) addWaiter() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.resolved {
		return false
	}
	e.waiters++
	return true
}

// removeWaiter 撤回一个等待者的需求登记
func (e *transcodeEntry) removeWaiter() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.waiters > 0 {
		e.waiters--
	}
}

// active 是否仍有等待者
func (e *transcodeEntry) active() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.waiters > 0
}

// waitersLocked 返回等待者计数。调用方必须已持有队列锁——等待者登记
// （Enqueue/Cancel）在队列锁下串行化，故该读数不会错过并发登记
func (e *transcodeEntry) waitersLocked() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.waiters
}

// finish 广播执行结果给所有等待者，并从队列映射中移除该任务
// （此后同 key 的新需求才允许重新入队执行）
func (q *TranscodeQueue) finish(entry *transcodeEntry, file appimage.File, err error) {
	entry.mu.Lock()
	if entry.resolved {
		entry.mu.Unlock()
		return
	}
	entry.resolved = true
	entry.file = file
	entry.err = err
	entry.mu.Unlock()
	close(entry.done)

	q.mu.Lock()
	// 仅当映射中仍是该 entry 时删除（防与并发 Enqueue 重试交错误删新 entry）
	if cur, ok := q.entries[entry.item.Key()]; ok && cur == entry {
		delete(q.entries, entry.item.Key())
	}
	q.mu.Unlock()
}

// result 读取执行结果
func (e *transcodeEntry) result() (appimage.File, error) {
	<-e.done
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.file, e.err
}

// TranscodeQueue 拉取式转码队列的内存实现：按 key 去重、按优先级阈值出队
type TranscodeQueue struct {
	mu      sync.Mutex
	entries map[string]*transcodeEntry
	// notify 有新任务入队时关闭并重建，唤醒阻塞在等待任务上的 Consume 循环
	notify chan struct{}
	closed bool
}

func NewTranscodeQueue() *TranscodeQueue {
	return &TranscodeQueue{
		entries: make(map[string]*transcodeEntry),
		notify:  make(chan struct{}),
	}
}

// Enqueue 登记一个变体转码需求。同 key 已有待执行任务时返回指向同一执行的等待者，
// 且该任务的优先级提升为两者中较高者
func (q *TranscodeQueue) Enqueue(ctx context.Context, item appimage.TranscodeItem) (appimage.TranscodeWaiter, error) {
	for {
		q.mu.Lock()
		if q.closed {
			q.mu.Unlock()
			return nil, context.Canceled
		}
		if entry, ok := q.entries[item.Key()]; ok {
			// 在持锁状态下登记等待者，避免与 Consume 的「取走并复查活跃」竞态窗口交错
			if !entry.addWaiter() {
				// 任务刚好在登记前被解决，从映射中移除并重试以获得新任务
				delete(q.entries, item.Key())
				q.mu.Unlock()
				continue
			}
			// 同 key 高优先级等待者到达 → 任务优先级提升。
			// 仅限未认领的任务：已交给 worker 执行的任务不再变更（Item() 无锁读）
			if !entry.claimed && item.Priority() > entry.item.Priority() {
				entry.item = item
			}
			q.mu.Unlock()
			return &transcodeWaiter{entry: entry}, nil
		}
		entry := &transcodeEntry{item: item, done: make(chan struct{})}
		entry.waiters = 1
		entry.queue = q
		q.entries[item.Key()] = entry
		close(q.notify)
		q.notify = make(chan struct{})
		q.mu.Unlock()
		return &transcodeWaiter{entry: entry}, nil
	}
}

// Consume 迭代产出优先级 ≥ minPriority 且仍有等待者的任务。任务交出后从队列移除；
// 结果通过 job.Resolve 广播。循环持续直到 ctx 取消
func (q *TranscodeQueue) Consume(ctx context.Context, minPriority appimage.Prio) iter.Seq2[appimage.TranscodeJob, error] {
	return func(yield func(appimage.TranscodeJob, error) bool) {
		for {
			if ctx.Err() != nil {
				return
			}
			q.mu.Lock()
			if q.closed {
				q.mu.Unlock()
				return
			}
			// 取走优先级最高且仍活跃的任务（未启动即撤光的任务在此被淘汰）。
			// 已被认领（执行中/已 yield）的任务跳过但不从映射删除——运行中的
			// key 保持可去重，直到 Resolve 后才允许同 key 重新入队
			var (
				bestEntry *transcodeEntry
			)
			for key, entry := range q.entries {
				if entry.claimed {
					continue
				}
				if entry.item.Priority() < minPriority {
					continue
				}
				if entry.waitersLocked() == 0 {
					// 需求已撤光的未启动任务：直接淘汰
					delete(q.entries, key)
					continue
				}
				if bestEntry == nil || entry.item.Priority() > bestEntry.item.Priority() {
					bestEntry = entry
				}
			}
			if bestEntry == nil {
				notify := q.notify
				q.mu.Unlock()
				select {
				case <-ctx.Done():
					return
				case <-notify:
				}
				continue
			}
			bestEntry.claimed = true
			q.mu.Unlock()

			job := &transcodeJob{entry: bestEntry}
			if !yield(job, nil) {
				return
			}
		}
	}
}

// transcodeJob worker 侧句柄
type transcodeJob struct {
	entry *transcodeEntry
}

func (j *transcodeJob) Item() appimage.TranscodeItem { return j.entry.item }

func (j *transcodeJob) Resolve(file appimage.File, err error) {
	j.entry.queue.finish(j.entry, file, err)
}

// transcodeWaiter 请求侧句柄
type transcodeWaiter struct {
	entry *transcodeEntry
}

func (w *transcodeWaiter) Wait(ctx context.Context) (appimage.File, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-w.entry.done:
	}
	return w.entry.result()
}

func (w *transcodeWaiter) Cancel() {
	w.entry.removeWaiter()
}
