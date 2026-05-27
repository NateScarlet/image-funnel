package util

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestKeyLock_Basic 验证基本加锁与解锁功能，以及不同 key 之间的互不干扰
func TestKeyLock_Basic(t *testing.T) {
	kl := &KeyLock{}

	// 同一个 key 在被锁定时，另一个 goroutine 获取锁应该会被阻塞
	kl.Lock("key1")
	locked := make(chan struct{})
	go func() {
		kl.Lock("key1")
		close(locked)
		kl.Unlock("key1")
	}()

	select {
	case <-locked:
		t.Fatal("key1 was locked but another goroutine obtained lock")
	case <-time.After(50 * time.Millisecond):
		// 正常阻塞
	}

	kl.Unlock("key1")

	select {
	case <-locked:
		// 释放后成功获得锁
	case <-time.After(100 * time.Millisecond):
		t.Fatal("lock was not released or other goroutine failed to obtain lock")
	}

	// 不同的 key 应该能够同时加锁，不互相阻塞
	kl.Lock("keyA")
	chanB := make(chan struct{})
	go func() {
		kl.Lock("keyB")
		close(chanB)
		kl.Unlock("keyB")
	}()

	select {
	case <-chanB:
		// 正常通过，不同的 key 互不阻塞
	case <-time.After(100 * time.Millisecond):
		t.Fatal("keyB was blocked by keyA")
	}
	kl.Unlock("keyA")
}

// TestKeyLock_DuplicateUnlock 验证多次解锁同一个 key 不会 panic 且可以安全忽略
func TestKeyLock_DuplicateUnlock(t *testing.T) {
	kl := &KeyLock{}

	kl.Lock("key_dup")
	kl.Unlock("key_dup")

	// 触发重复解锁，新设计中不应发生 panic
	kl.Unlock("key_dup")
	kl.Unlock("key_dup")
}

// TestKeyLock_Concurrency 压力测试高并发下加解锁的正确性
func TestKeyLock_Concurrency(t *testing.T) {
	kl := &KeyLock{}
	var wg sync.WaitGroup
	var counter int64

	const goroutines = 100
	const iterations = 1000
	const key = "concurrency_key"

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				kl.Lock(key)
				// 临界区逻辑
				atomic.AddInt64(&counter, 1)
				time.Sleep(time.Nanosecond) // 略微让出 CPU
				kl.Unlock(key)
			}
		}()
	}

	wg.Wait()
	if counter != goroutines*iterations {
		t.Errorf("expected counter %d, got %d", goroutines*iterations, counter)
	}
}

// BenchmarkKeyLock 测试不同 key 数量下的性能
func BenchmarkKeyLock(b *testing.B) {
	for _, keyCount := range []int{1, 10, 100} {
		b.Run("keys="+itoa(keyCount), func(b *testing.B) {
			kl := &KeyLock{}
			keys := make([]string, keyCount)
			for i := range keyCount {
				keys[i] = "key_" + itoa(i)
			}

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				var idx int
				for pb.Next() {
					key := keys[idx%len(keys)]
					kl.Lock(key)
					kl.Unlock(key)
					idx++
				}
			})
		})
	}
}

// BenchmarkMutexBaseline 提供 sync.Mutex 作为基线，用于对比性能差距
func BenchmarkMutexBaseline(b *testing.B) {
	var mu sync.Mutex
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			_ = 1
			mu.Unlock()
		}
	})
}

// 辅助函数，将整数转为字符串，避免引入 strconv 开销
func itoa(i int) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	var buf [8]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = digits[i%10]
		i /= 10
	}
	return string(buf[pos:])
}


