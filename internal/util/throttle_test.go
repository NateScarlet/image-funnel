package util

import (
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
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for unthrottled event")
	}

	close(inCh)
}

func TestThrottleBy_WithThrottle(t *testing.T) {
	type event struct {
		key string
		val int
	}

	inCh := make(chan event, 100)
	var seq iter.Seq2[event, error] = func(yield func(event, error) bool) {
		for ev := range inCh {
			if !yield(ev, nil) {
				return
			}
		}
	}

	throttleTime := 40 * time.Millisecond
	results := ThrottleBy(seq, throttleTime, func(ev event, err error) string {
		return ev.key
	})

	received := make(chan event, 100)
	go func() {
		for ev, _ := range results {
			received <- ev
		}
	}()

	// T0: 发送 key_a. 应当立即收到 (Leading)
	inCh <- event{key: "key_a", val: 1}
	select {
	case ev := <-received:
		if ev.key != "key_a" || ev.val != 1 {
			t.Errorf("T0: expected key_a.val=1, got %v", ev)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("T0: timeout waiting for leading edge of key_a")
	}

	// T0 + 10ms: 在 key_a 的冷却期内发送 key_a. 应当被缓存，不立即触发
	time.Sleep(10 * time.Millisecond)
	inCh <- event{key: "key_a", val: 2}
	select {
	case ev := <-received:
		t.Fatalf("T0+10ms: unexpected event: %v", ev)
	default:
		// 正常
	}

	// T0 + 15ms: 发送不同的键 key_b. 应当立即收到 (Leading，因为键不同，节流独立)
	time.Sleep(5 * time.Millisecond)
	inCh <- event{key: "key_b", val: 100}
	select {
	case ev := <-received:
		if ev.key != "key_b" || ev.val != 100 {
			t.Errorf("T0+15ms: expected key_b.val=100, got %v", ev)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("T0+15ms: timeout waiting for leading edge of key_b")
	}

	// T0 + 20ms: 再次在 key_a 的冷却期内发送 key_a. 应当覆盖挂起值，此时 pending 值为 val: 3
	time.Sleep(5 * time.Millisecond)
	inCh <- event{key: "key_a", val: 3}
	select {
	case ev := <-received:
		t.Fatalf("T0+20ms: unexpected event: %v", ev)
	default:
		// 正常
	}

	// T0 + 55ms: 冷却期结束（T0 + 40ms 以后）。应当收到最新的 key_a 变更 (Trailing, val: 3)
	time.Sleep(35 * time.Millisecond)
	select {
	case ev := <-received:
		if ev.key != "key_a" || ev.val != 3 {
			t.Errorf("T0+55ms: expected key_a.val=3 (trailing), got %v", ev)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("T0+55ms: timeout waiting for trailing edge of key_a")
	}

	// 确认此后无任何多余事件
	select {
	case ev := <-received:
		t.Fatalf("unexpected event: %v", ev)
	default:
		// 正常
	}

	close(inCh)
}
