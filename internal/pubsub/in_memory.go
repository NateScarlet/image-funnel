package pubsub

import (
	"context"
	"fmt"
	"iter"
	"main/internal/forked/container/list"
	"main/internal/forked/container/ring"
	"runtime"
	"slices"
	"sync/atomic"
	"time"
	"unsafe"
)

func NewInMemoryTopic[T any](options ...InMemoryTopicOption) (*InMemoryTopic[T], func()) {
	const maxSize = 128
	if size := unsafe.Sizeof(*new(T)); size > maxSize {
		panic(fmt.Sprintf("topic content too large (%d bytes), use pointer instead", size))
	}
	var done = make(chan struct{})
	var opts = newInMemoryTopicOptions(options...)
	var topic = &InMemoryTopic[T]{
		embedInMemoryTopicOptions: *opts,
		publish:                   make(chan T, opts.publishBuffer),
		subscribe:                 make(chan *inMemorySubscription[T]),
		done:                      done,
		rescaleRequest:            make(chan struct{}, 1),
		buf:                       &ring.Ring[inMemoryTopicEvent[T]]{},
		bufLen:                    1,
	}
	topic.rescale()
	go topic.loop()
	return topic, func() {
		close(done)
	}
}

var _ Topic[any] = (*InMemoryTopic[any])(nil)

type embedInMemoryTopicOptions = InMemoryTopicOptions

// InMemoryTopic 专门为网页实时更新所使用的 graphql subscription 优化。
// 新订阅者只会接收到新事件，不会接收到历史事件（因为最新状态直接刷新页面就行）。
type InMemoryTopic[T any] struct {
	embedInMemoryTopicOptions

	lastIndex      atomic.Uint64
	done           <-chan struct{}
	rescaleRequest chan struct{}
	publish        chan T
	subscribe      chan *inMemorySubscription[T]
	buf            *ring.Ring[inMemoryTopicEvent[T]] // 即将写入的位置
	bufLen         int
	shards         []*inMemoryTopicShard[T]
}

//go:norace
func (t *InMemoryTopic[T]) earliestEvent(after uint64) (*ring.Ring[inMemoryTopicEvent[T]], uint64, T) {
	// 只访问水位线上的元素，不存在并发缩容
	var cursor = t.buf // 无并发发布时，主循环指针直接就是尾部
	var index, value = cursor.Value.Read()
	for index > after+1 {
		// 事件被并发覆盖(不再是尾部)，需要尝试向前回溯一整圈
		// 因为只能访问水位线上的元素，所以虽然向后更近但是不能向后
		var prev = cursor.Prev()
		var prevIndex, prevValue = prev.Value.Read()
		if prevIndex == index-1 {
			index = prevIndex
			value = prevValue
			cursor = prev
		} else {
			break
		}
	}
	return cursor, index, value
}

// inMemoryTopicEvent 使用函数避免结构体的原子写入问题
type inMemoryTopicEvent[T any] func() (uint64, T)

func (f inMemoryTopicEvent[T]) Read() (uint64, T) {
	if f == nil {
		var zero T
		return 0, zero
	}
	return f()
}

func newInMemoryTopicEvent[T any](index uint64, value T) inMemoryTopicEvent[T] {
	return func() (uint64, T) {
		return index, value
	}
}

//go:norace
func (t *InMemoryTopic[T]) unsafePush(v T, watermark uint64) {
	var index = t.lastIndex.Add(1)
	t.buf.Value = newInMemoryTopicEvent(index, v)
	var nextIndex, _ = t.buf.Next().Value.Read()
	if t.bufLen < t.capacity && nextIndex > watermark {
		// 缓冲区不够大，进行扩充
		t.buf.Link(&ring.Ring[inMemoryTopicEvent[T]]{})
		t.bufLen++
	}
	t.buf = t.buf.Next()
}

func (t *InMemoryTopic[T]) unsafeShrinkBuffer(watermark uint64) {
	if watermark == 0 {
		return
	}
	var nextIndex, _ = t.buf.Value.Read()
	if nextIndex == 0 {
		// 还处于初始状态，未写入
		return
	}
	// 水位线肯定在最早事件序号以上
	var underwater = int(watermark - nextIndex)
	// 每次最多减小到一半
	var wantLen = max(t.bufLen/2, t.bufLen-underwater)
	if wantLen <= 1 {
		// 不能缩了
		return
	}
	// Unlink 是从下一个节点开始移除，不会移除到订阅者的 cursor (最快的订阅者也只可能是 t.buf)
	t.buf.Unlink(t.bufLen - wantLen)
	t.bufLen = wantLen
}

// watermark 返回整个主题的水位线，由于并发相比实际水位会更低
func (t *InMemoryTopic[T]) watermark() uint64 {
	var watermark uint64
	for index, shard := range t.shards {
		var v = shard.watermark.Load()
		if index == 0 {
			watermark = v
		} else {
			watermark = min(watermark, v)
		}
	}
	return watermark
}

func (t *InMemoryTopic[T]) loop() {
	var idleTimer = time.NewTimer(t.idleTimeout)
	defer idleTimer.Stop()
	var rescaleTicker = time.NewTicker(t.rescaleInterval)
	defer rescaleTicker.Stop()
	for {
		select {
		case <-t.done:
			return
		case <-t.rescaleRequest:
			t.rescale()
		case <-rescaleTicker.C:
			t.rescale()
		case v := <-t.publish:
			// 串行发布，不存在并发
			var watermark = t.watermark()
			var batchStartAt = time.Now()
			var batchSize = 1
			t.unsafePush(v, watermark)
			// 连续的发布仅唤醒一次
		batchLoop:
			for batchSize < t.capacity && time.Since(batchStartAt) < t.maxBatchWait {
				select {
				case <-t.done:
					return
				case v := <-t.publish:
					t.unsafePush(v, watermark)
					batchSize++
				default:
					break batchLoop
				}
			}
			// 唤醒所有分片
			for _, shard := range t.shards {
				select {
				case shard.notify <- struct{}{}:
				default:
				}
			}
			idleTimer.Reset(t.idleTimeout)
		case <-idleTimer.C:
			t.unsafeShrinkBuffer(t.watermark())
			if len(t.shards) < cap(t.shards)/2 {
				t.shards = slices.Clone(t.shards)
			}
			if t.bufLen == 1 && cap(t.shards) <= 2 {
				idleTimer.Stop()
			} else {
				idleTimer.Reset(t.idleTimeout)
			}
		}
	}
}

func (t *InMemoryTopic[T]) rescale() {
	type shardSnapshot struct {
		index             int
		shard             *inMemoryTopicShard[T]
		subscriptionCount uint64
	}
	var totalSubscriptionCount uint64
	var snapshot = slices.AppendSeq(make([]shardSnapshot, 0, len(t.shards)), func(yield func(shardSnapshot) bool) {
		for index, i := range t.shards {
			if i.didStop.Load() {
				continue
			}
			var subscriptionCount = i.subscriptionCount.Load()
			totalSubscriptionCount += subscriptionCount
			if !yield(shardSnapshot{
				index,
				i,
				subscriptionCount,
			}) {
				return
			}
		}
	})
	slices.SortFunc(snapshot, func(a, b shardSnapshot) int {
		if a.subscriptionCount > b.subscriptionCount {
			return -1
		}
		if a.subscriptionCount < b.subscriptionCount {
			return 1
		}
		return 0
	})
	var shouldUpdateShards = len(snapshot) != len(t.shards)
	if !shouldUpdateShards {
		for index, i := range snapshot {
			if i.index != index {
				shouldUpdateShards = true
				break
			}
		}
	}
	if shouldUpdateShards {
		t.shards = slices.AppendSeq(t.shards[:0], func(yield func(*inMemoryTopicShard[T]) bool) {
			for _, i := range snapshot {
				if !yield(i.shard) {
					return
				}
			}
		})
	}

	// 新增分片
	var wantShard = int(totalSubscriptionCount/uint64(t.targetShardSize)) + 1
	if t.maxShards > 0 {
		wantShard = min(t.maxShards, wantShard)
	}
	for len(t.shards) < wantShard {
		t.shards = append(t.shards, t.newShard())
	}
	// 设置容量
	var avgSize = totalSubscriptionCount / uint64(wantShard)
	for index, shard := range t.shards {
		var capacity int64
		if index >= wantShard {
			// 停止多余的分片
			capacity = -2
		} else if index >= wantShard/2 {
			// 一半是无限的，保证新订阅不阻塞
			capacity = -1
		} else {
			// 其余按平均分配
			capacity = int64(avgSize)
		}
		if shard.capacity.Swap(capacity) != capacity {
			select {
			case shard.reload <- struct{}{}:
			default:
			}
		}
	}
}

func (t *InMemoryTopic[T]) Publish(ctx context.Context, o T, options ...PublishOption) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.done:
		return ErrTopicDisposed
	case t.publish <- o:
		return nil
	}
}

// Subscribe 阻塞获取新事件，不会获取到订阅前产生的事件
func (t *InMemoryTopic[T]) Subscribe(ctx context.Context) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var after = t.lastIndex.Load()
		var notify = make(chan struct{}, 1)
		var sub = &inMemorySubscription[T]{
			notify: notify,
			cursor: t.buf,
		}
		sub.after.Store(after)
		defer sub.setFlag(subscriptionDidStop)
		var zero T
		// 注册
		select {
		case <-ctx.Done():
			yield(zero, ctx.Err())
			return
		case <-t.done:
			yield(zero, ErrTopicDisposed)
			return
		case t.subscribe <- sub:
		}
		// 监听
		for {
			select {
			case <-ctx.Done():
				yield(zero, ctx.Err())
				return
			case <-t.done:
				yield(zero, ErrTopicDisposed)
				return
			case <-notify:
				sub.clearFlag(subscriptionBusy)
				for i, err := range sub.more(t) {
					if ctx.Err() != nil {
						yield(zero, err)
						return
					}
					if !yield(i, err) {
						return
					}
				}
			}
		}
	}
}

func (t *InMemoryTopic[T]) newShard() *inMemoryTopicShard[T] {
	var shard = &inMemoryTopicShard[T]{
		notify: make(chan struct{}, 1),
		reload: make(chan struct{}, 1),
	}
	shard.capacity.Store(-1)
	go shard.loop(t)
	return shard
}

type inMemoryTopicShard[T any] struct {
	subscriptions []*inMemorySubscription[T]
	free          list.List[int]
	notify        chan struct{}
	reload        chan struct{}
	// 水位线，最低的订阅者 after，水位线以下的事件可安全丢弃
	watermark         atomic.Uint64
	subscriptionCount atomic.Uint64
	// -1: 无限
	// -2: 订阅者归零时停止
	capacity atomic.Int64
	didStop  atomic.Bool
}

func (shard *inMemoryTopicShard[T]) loop(t *InMemoryTopic[T]) {
	defer func() {
		shard.didStop.Store(true)
		select {
		case t.rescaleRequest <- struct{}{}:
		default:
		}
	}()
	var idleTimer = time.NewTimer(t.idleTimeout)
	defer idleTimer.Stop()
	var capacity int
	var subscribe = t.subscribe
	var reload = func() {
		capacity = int(shard.capacity.Load())
		if capacity == -1 || (capacity > 0 && shard.subscriptionCount.Load() < uint64(capacity)) {
			subscribe = t.subscribe
		} else {
			subscribe = nil
		}
	}
	for {
		select {
		case <-t.done:
			return
		case <-idleTimer.C:
			reload()

			// 释放内存
			var empty bool
			if wantCap := cap(shard.subscriptions) / 2; len(shard.subscriptions) == 0 || len(shard.subscriptions) < wantCap {
				if len(shard.subscriptions) == 0 {
					shard.subscriptions = nil
				} else {
					shard.subscriptions = slices.AppendSeq(
						make([]*inMemorySubscription[T], 0, wantCap),
						func(yield func(*inMemorySubscription[T]) bool) {
							for _, i := range shard.subscriptions {
								if i.hasFlag(subscriptionInFreeList | subscriptionDidStop) {
									continue
								}
								if !yield(i) {
									return
								}
							}
						},
					)
				}
				empty = len(shard.subscriptions) == 0
				shard.subscriptionCount.Store(uint64(len(shard.subscriptions)))
				shard.free = list.List[int]{} // 重置空闲位置
			}
			if empty {
				if capacity == -2 {
					return
				}
				idleTimer.Stop()
			} else {
				idleTimer.Reset(t.idleTimeout)
			}
		case sub := <-subscribe:
			// 竞争接收新订阅，只有一个分片会被唤醒
			if el := shard.free.Front(); el != nil {
				shard.subscriptions[el.Value] = sub
				shard.free.Remove(el)
			} else {
				shard.subscriptions = append(shard.subscriptions, sub)
			}
			var count = int(shard.subscriptionCount.Add(1))
			if count == t.targetShardSize || count == capacity {
				// 只在抵达设定值的时候触发一次
				// rescale 之后如果设定值均低于当前，则由其他分片触发
				select {
				case t.rescaleRequest <- struct{}{}:
				default:
				}
			}
			reload()
			idleTimer.Reset(t.idleTimeout)
		case v := <-shard.notify:
			// 接收新事件通知，所有分片都会被唤醒
			var watermark uint64
			var didChange bool
			var subscriptionCount uint64
			for index, i := range shard.subscriptions {
				// 忽略已停止订阅
				flags := i.getFlags()
				if flags&subscriptionDidStop != 0 {
					if flags&subscriptionInFreeList == 0 {
						shard.free.PushBack(index)
						i.setFlag(subscriptionInFreeList)
						didChange = true
					}
					continue
				}

				// 统计
				var after = i.after.Load()
				if subscriptionCount == 0 {
					watermark = after
				} else {
					watermark = min(watermark, after)
				}
				subscriptionCount++

				// 通知
				if flags&subscriptionBusy == 0 {
					select {
					case i.notify <- v:
					default:
						// 非阻塞发送，慢订阅者不影响其他订阅者
						i.setFlag(subscriptionBusy)
					}
				}
			}
			shard.subscriptionCount.Store(subscriptionCount)
			shard.watermark.Store(watermark)
			if subscriptionCount == 0 && capacity == -2 {
				return
			}
			if didChange {
				idleTimer.Reset(t.idleTimeout)
			}
		case <-shard.reload:
			reload()
			if capacity == -2 && shard.subscriptionCount.Load() == 0 {
				return
			}
		}
	}
}

type inMemorySubscription[T any] struct {
	notify chan<- struct{}
	// 读取指针，由水位线保证不会被并发移除
	// 可能并发写入覆盖的事件，使用事件前应复制再检查序号
	cursor *ring.Ring[inMemoryTopicEvent[T]]
	after  atomic.Uint64
	flags  atomic.Uint32
}

type inMemorySubscriptionFlag int

const (
	subscriptionDidStop inMemorySubscriptionFlag = 1 << iota
	subscriptionBusy
	subscriptionInFreeList
)

func (s *inMemorySubscription[T]) hasFlag(flag inMemorySubscriptionFlag) bool {
	return s.flags.Load()&uint32(flag) != 0
}

func (s *inMemorySubscription[T]) getFlags() inMemorySubscriptionFlag {
	return inMemorySubscriptionFlag(s.flags.Load())
}

func (s *inMemorySubscription[T]) setFlag(flag inMemorySubscriptionFlag) {
	if flag == 0 {
		return
	}

	mask := uint32(flag)
	// 原子设置位：flags = flags | mask
	s.flags.Or(mask)
}

func (s *inMemorySubscription[T]) clearFlag(flag inMemorySubscriptionFlag) {
	if flag == 0 {
		return
	}

	mask := uint32(flag)
	// 原子清除位：flags = flags &^ mask
	s.flags.And(^mask)
}

//go:norace
func (s *inMemorySubscription[T]) next(cursor *ring.Ring[inMemoryTopicEvent[T]], after uint64) (*ring.Ring[inMemoryTopicEvent[T]], uint64, T) {
	var next = cursor.Next()
	var index, value = next.Value.Read()
	if index != after+1 {
		var zero T
		return next, 0, zero
	}
	return next, index, value
}

func (s *inMemorySubscription[T]) more(topic *InMemoryTopic[T]) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		// 主循环修改的是水下的未使用部分，不存在并发
		var cursor = s.cursor
		var index, value = cursor.Value.Read()
		var after = s.after.Load()
		var err error
		if index > after+1 {
			// 有事件丢失，重置指针到最早可用事件
			cursor, index, value = topic.earliestEvent(after)
			// 由于订阅时获取 after 和 cursor 不是原子操作，所以可能重置后发现没丢失
			if index > after+1 {
				var dropped = index - after - 1
				err = fmt.Errorf("%w (%d dropped)", ErrUndeliveredEvents, dropped)
			}
			after = index - 1
		}
		// 获取所有连续新事件
		for index == after+1 {
			if !yield(value, err) {
				return
			}
			err = nil
			after = index
			s.after.Store(after)
			cursor, index, value = s.next(cursor, after)
			s.cursor = cursor
		}
	}
}

type InMemoryTopicOptions struct {
	capacity        int
	publishBuffer   int
	maxBatchWait    time.Duration
	idleTimeout     time.Duration
	targetShardSize int
	maxShards       int
	rescaleInterval time.Duration
}

func newInMemoryTopicOptions(options ...InMemoryTopicOption) *InMemoryTopicOptions {
	var opts = new(InMemoryTopicOptions)
	opts.capacity = 1024
	opts.maxBatchWait = 5 * time.Millisecond
	opts.publishBuffer = 16
	opts.idleTimeout = 10 * time.Minute
	opts.targetShardSize = 512
	opts.rescaleInterval = time.Minute
	opts.maxShards = max(8, runtime.GOMAXPROCS(0)*2)
	for _, i := range options {
		i(opts)
	}
	if opts.capacity <= 0 {
		panic("capacity must be positive")
	}
	return opts
}

type InMemoryTopicOption func(opts *InMemoryTopicOptions)

// InMemoryTopicWithMaxShards 决定最多分割的分片数量，不会预分配，闲置 Topic 只会有一个分片。
func InMemoryTopicWithMaxShards(maxShards int) InMemoryTopicOption {
	return func(opts *InMemoryTopicOptions) {
		opts.maxShards = maxShards
	}
}

func InMemoryTopicWithTargetShardSize(targetShardSize int) InMemoryTopicOption {
	if targetShardSize <= 0 {
		panic("target shard size must be positive")
	}
	return func(opts *InMemoryTopicOptions) {
		opts.targetShardSize = targetShardSize
	}
}

// InMemoryTopicWithPublishBuffer 不应太高，发送太快订阅者也处理不过来
func InMemoryTopicWithPublishBuffer(publishBuffer int) InMemoryTopicOption {
	if publishBuffer < 0 {
		panic("publish buffer can not be negative")
	}
	return func(opts *InMemoryTopicOptions) {
		opts.publishBuffer = publishBuffer
	}
}

// InMemoryTopicWithCapacity 用于处理发布比消费更快的情况，如果差距超过 capacity 会报错事件丢失（可忽略）
func InMemoryTopicWithCapacity(capacity int) InMemoryTopicOption {
	return func(opts *InMemoryTopicOptions) {
		opts.capacity = capacity
	}
}

// InMemoryTopicWithMaxBatchWait 不是 minWait，读尽就直接发送不会等待，大多数情况不会导致延迟
func InMemoryTopicWithMaxBatchWait(maxBatchWait time.Duration) InMemoryTopicOption {
	return func(opts *InMemoryTopicOptions) {
		opts.maxBatchWait = maxBatchWait
	}
}

// Deprecated: renamed to [InMemoryTopicWithCapacity]
func InMemoryTopicOptionCapacity(v int) InMemoryTopicOption {
	return func(opts *InMemoryTopicOptions) {
		opts.capacity = v
	}
}
