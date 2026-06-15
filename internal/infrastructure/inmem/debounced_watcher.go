package inmem

import (
	"context"
	"iter"
	"main/internal/domain/directory"
	"main/internal/shared"
	"sync"
	"time"
)

type debouncedEvent struct {
	timer  *time.Timer
	change *directory.FileChange
}

// DebouncedWatcher 装饰器 - 文件变更防抖监听器
type DebouncedWatcher struct {
	next     directory.Watcher
	debounce time.Duration
}

// NewDebouncedWatcher 创建一个防抖监听器装饰实例
func NewDebouncedWatcher(next directory.Watcher, debounce time.Duration) *DebouncedWatcher {
	return &DebouncedWatcher{
		next:     next,
		debounce: debounce,
	}
}

// Watch 监听指定目录的文件变更，并对连续的创建与修改事件执行指定时长的防抖合并，删除与重命名则直接触发并立即清理防抖。
func (w *DebouncedWatcher) Watch(ctx context.Context, dir string) iter.Seq2[*directory.FileChange, error] {
	return func(yield func(*directory.FileChange, error) bool) {
		var mu sync.Mutex
		debounces := make(map[string]*debouncedEvent)

		// 异步事件传输管道
		outChan := make(chan struct {
			change *directory.FileChange
			err    error
		})

		subCtx, subCancel := context.WithCancel(ctx)

		go func() {
			defer subCancel()
			for change, err := range w.next.Watch(subCtx, dir) {
				if err != nil {
					select {
					case <-subCtx.Done():
					case outChan <- struct {
						change *directory.FileChange
						err    error
					}{nil, err}:
					}
					return
				}

				mu.Lock()
				absPath := change.AbsPath()
				action := change.Action()

				// 如果是删除或重命名操作，立即清除挂起的创建/修改防抖，并立即发送
				if action == shared.FileActionRemove || action == shared.FileActionRename {
					if d, ok := debounces[absPath]; ok {
						d.timer.Stop()
						delete(debounces, absPath)
					}
					mu.Unlock()

					select {
					case <-subCtx.Done():
						return
					case outChan <- struct {
						change *directory.FileChange
						err    error
					}{change, nil}:
					}
					continue
				}

				// 创建或更新操作，进行防抖处理
				if d, ok := debounces[absPath]; ok {
					d.timer.Stop()
				}

				d := &debouncedEvent{
					change: change,
				}
				d.timer = time.AfterFunc(w.debounce, func() {
					mu.Lock()
					defer mu.Unlock()

					// 确认当前事件未在延时期间被新的变更事件覆盖或被删除事件取消
					current, ok := debounces[absPath]
					if ok && current == d {
						delete(debounces, absPath)
						select {
						case <-subCtx.Done():
						case outChan <- struct {
							change *directory.FileChange
							err    error
						}{change, nil}:
						}
					}
				})
				debounces[absPath] = d
				mu.Unlock()
			}
		}()

		// 确保迭代生命周期结束时，子协程能安全销毁，定时器得以正确清理
		defer func() {
			subCancel()
			mu.Lock()
			for _, d := range debounces {
				d.timer.Stop()
			}
			mu.Unlock()
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case res, ok := <-outChan:
				if !ok {
					return
				}
				if res.err != nil {
					if !yield(nil, res.err) {
						return
					}
					return
				}
				if !yield(res.change, nil) {
					return
				}
			}
		}
	}
}

// 确保实现了 Watcher 接口
var _ directory.Watcher = (*DebouncedWatcher)(nil)
