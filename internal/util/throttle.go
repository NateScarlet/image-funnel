package util

import (
	"context"
	"iter"
	"time"
)

// ThrottleBy 对 iter.Seq2[K, V] 输入流根据给定的 keyOf 获取的键 T 进行节流（Throttle）。
// 它实现了针对同一个键值的 leading 与 trailing 双边缘触发的节流效果：
// 1. 若当前键值不在冷却期中，收到元素时立即发送（Leading 触发），并启动冷却定时器；
// 2. 处于冷却期时接收到的元素只暂存最新值；
// 3. 冷却定时器到期时，若存在被暂存的最新值，则发送该值（Trailing 触发），并重新启动冷却定时器；
// 4. 不同键值之间的冷却定时器彼此独立，互不干扰；
// 5. 在输入流遍历结束或者退出拉取时，会自动安全回收并停止所有运行中的定时器与后台协程。
func ThrottleBy[K any, V any, T comparable](seq iter.Seq2[K, V], throttle time.Duration, keyOf func(K, V) T) iter.Seq2[K, V] {
	if throttle <= 0 {
		return seq
	}

	return func(yield func(K, V) bool) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		type throttleResult struct {
			k K
			v V
		}

		type timerExpired struct {
			key T
		}

		outCh := make(chan throttleResult)
		expiredCh := make(chan timerExpired, 100)
		eventCh := make(chan throttleResult)

		// 启动拉取输入序列的协程
		go func() {
			defer close(eventCh)
			for k, v := range seq {
				select {
				case eventCh <- throttleResult{k: k, v: v}:
				case <-ctx.Done():
					return
				}
			}
		}()

		// 启动主节流控制协程
		go func() {
			defer close(outCh)

			type keyState struct {
				timer      *time.Timer
				pendingVal *throttleResult
			}
			states := make(map[T]*keyState)

			// 退出时正确 Stop 所有的 Timer，防止定时器和协程泄露
			defer func() {
				for _, state := range states {
					if state.timer != nil {
						state.timer.Stop()
					}
				}
			}()

			for {
				select {
				case <-ctx.Done():
					return
				case item, ok := <-eventCh:
					if !ok {
						return
					}

					key := keyOf(item.k, item.v)
					state, exists := states[key]
					if !exists {
						state = &keyState{}
						states[key] = state
					}

					if state.timer == nil {
						// 处于冷却期外：立即触发 (Leading)
						select {
						case outCh <- item:
						case <-ctx.Done():
							return
						}

						// 开启冷却定时器
						targetKey := key
						state.timer = time.AfterFunc(throttle, func() {
							select {
							case expiredCh <- timerExpired{key: targetKey}:
							case <-ctx.Done():
							}
						})
					} else {
						// 处于冷却期内：只更新缓存的最新的值
						state.pendingVal = &item
					}

				case exp := <-expiredCh:
					state, exists := states[exp.key]
					if !exists || state.timer == nil {
						continue
					}

					state.timer = nil

					// 检查是否有挂起值需要发送 (Trailing 触发)
					if state.pendingVal != nil {
						val := *state.pendingVal
						state.pendingVal = nil

						select {
						case outCh <- val:
						case <-ctx.Done():
							return
						}

						// 既然触发了 Trailing，需要进入下一轮冷却状态
						targetKey := exp.key
						state.timer = time.AfterFunc(throttle, func() {
							select {
							case expiredCh <- timerExpired{key: targetKey}:
							case <-ctx.Done():
							}
						})
					}
				}
			}
		}()

		// 主迭代循环拉取 outCh 并 yield
		for res := range outCh {
			if !yield(res.k, res.v) {
				return
			}
		}
	}
}
