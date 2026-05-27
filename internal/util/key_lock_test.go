package util

import (
	"sync"
	"testing"
)

// BenchmarkKeyLock 测试不同 key 数量下的性能
func BenchmarkKeyLock(b *testing.B) {
	// 定义测试的 key 池大小：1（高竞争）、10（中竞争）、100（低竞争）
	for _, keyCount := range []int{1, 10, 100} {
		b.Run("keys="+itoa(keyCount), func(b *testing.B) {
			kl := &KeyLock{}
			// 预先生成 key 列表，避免运行时产生字符串分配开销
			keys := make([]string, keyCount)
			for i := range keyCount {
				keys[i] = "key_" + itoa(i)
			}

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				// 每个 goroutine 独立取一个递增的计数器，用于选择 key
				var idx int
				for pb.Next() {
					key := keys[idx%len(keys)]
					unlock := kl.Lock(key)
					// 模拟临界区内的轻微操作（可省略，仅测试锁开销）
					// 这里不加入实际工作负载，以测量锁本身的延迟
					unlock()
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
	// 简单实现，仅支持小范围整数，满足测试需求
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
