package util

import (
	"fmt"
	"iter"
	"testing"
	"time"
)

func TestThrottleBy_NoThrottle(t *testing.T) {
	inCh := make(chan int, 100)
	var seq iter.Seq2[int, error] = func(yield func(int, error) bool) {
		for val := range inCh {
			if !yield(val, nil) {
				return
			}
		}
	}

	// 0 延时，透传
	results := ThrottleBy(seq, 0, func(v int, err error) string {
		return "key"
	})

	received := make(chan int, 100)
	go func() {
		for v, _ := range results {
			received <- v
		}
	}()

	inCh <- 42
	select {
	case v := <-received:
		if v != 42 {
			t.Errorf("expected 42, got %d", v)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for unthrottled event")
	}

	close(inCh)
}

// TestThrottleBy_WithThrottle 覆盖 ThrottleBy 的冷却语义：
// 冷却窗口外的 leading 立即输出、冷却窗口内的事件只更新缓存的最新值、
// 不同键的冷却窗口彼此独立、窗口到期后输出缓存值的 trailing。
//
// 断言不依赖墙钟推算的时间点，而是依赖事件消费的因果顺序：节流协程按序消费输入，
// 而全新键的事件一定走 leading 立即输出，因此收到某个探针事件即证明它之前发送的
// 事件都已经被消费（写入缓存或已输出）。这样即使在负载很高的 CI 上出现调度延迟，
// 断言的语义依然成立。
func TestThrottleBy_WithThrottle(t *testing.T) {
	type event struct {
		key string
		val int
	}

	inCh := make(chan event, 100)
	t.Cleanup(func() { close(inCh) })
	var seq iter.Seq2[event, error] = func(yield func(event, error) bool) {
		for ev := range inCh {
			if !yield(ev, nil) {
				return
			}
		}
	}

	// 节流窗口远大于测试期间的喂入间隔与总喂入时长，
	// 保证整个喂入过程都落在同一个冷却窗口内。
	const throttleTime = 500 * time.Millisecond
	results := ThrottleBy(seq, throttleTime, func(ev event, err error) string {
		return ev.key
	})

	received := make(chan event, 100)
	go func() {
		for ev, _ := range results {
			received <- ev
		}
	}()

	// expectNext 断言下一个输出事件
	expectNext := func(want event) {
		t.Helper()
		select {
		case got := <-received:
			if got != want {
				t.Fatalf("expected %v, got %v", want, got)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for %v", want)
		}
	}

	// expectNothing 断言此刻没有任何待输出的事件
	expectNothing := func() {
		t.Helper()
		select {
		case got := <-received:
			t.Fatalf("unexpected event: %v", got)
		default:
		}
	}

	// probe 用一个全新的键发送事件作为同步点：全新键没有冷却状态，一定立即 leading 输出，
	// 且不会产生 trailing 事件。
	probe := func(val int) {
		t.Helper()
		ev := event{key: fmt.Sprintf("key_probe_%d", val), val: val}
		inCh <- ev
		expectNext(ev)
	}

	// 冷却窗口外：首个事件应立即触发 leading
	inCh <- event{key: "key_a", val: 1}
	expectNext(event{key: "key_a", val: 1})

	// 以远小于冷却窗口的间隔持续喂入 key_a：整个过程中都应处于冷却窗口内，
	// 只更新缓存的最新值而不输出任何事件。
	const lastVal = 11
	for val := 2; val <= lastVal; val++ {
		inCh <- event{key: "key_a", val: val}
		// 穿插另一个全新键，验证不同键的冷却窗口彼此独立
		if val == 6 {
			probe(100)
		}
		time.Sleep(20 * time.Millisecond)
	}

	// 探针同步证明最后一个 key_a 事件已被缓存，此时仍在冷却窗口内，不应有任何输出
	probe(101)
	expectNothing()

	// 冷却窗口到期：应触发 trailing，输出缓存中的最新值
	expectNext(event{key: "key_a", val: lastVal})

	// trailing 之后开启新的冷却窗口，且没有新的缓存值，不应再有输出
	select {
	case got := <-received:
		t.Fatalf("unexpected event after trailing edge: %v", got)
	case <-time.After(throttleTime + 100*time.Millisecond):
	}
}
