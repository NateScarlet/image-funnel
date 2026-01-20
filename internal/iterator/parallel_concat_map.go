package iterator

import (
	"context"
	"fmt"
	"iter"
	"slices"
	"sync"
)

func parallelConcatMap2[K1, V1, K2, V2 any](
	ctx context.Context,
	limit int,
	seq iter.Seq2[K1, V1],
	yield func(K2, V2) bool,
	project func(ctx context.Context, yield func(K2, V2) bool, k K1, v V1) bool, // 对外套一层调用，允许根据 seq 和 yield 推导类型
) bool {
	if limit == 1 {
		for k, v := range seq {
			if !project(ctx, yield, k, v) {
				return false
			}
		}
		return true
	}
	// 如果 project 是确定性的，则并发时的输出顺序应该和 limit==1 时一致

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type outputKind int
	const (
		data outputKind = iota
		complete
		stop
	)

	type output struct {
		kind  outputKind
		index int
		key   K2
		value V2
	}
	var out = make(chan output)
	go func() {
		defer close(out)
		var wg = new(sync.WaitGroup)
		func() {
			type input struct {
				index int
				key   K1
				value V1
			}
			var in = make(chan input)
			defer close(in)
			var nextIndex int
			var workerCount int
			for k, v := range seq {
				if limit <= 0 || workerCount < limit {
					select {
					case <-ctx.Done():
						return
					case in <- input{nextIndex, k, v}:
						nextIndex++
					default:
						// 开启新的 worker 再发送
						wg.Add(1)
						workerCount++
						go func() {
							defer wg.Done()
							for i := range in {
								var ok = project(
									ctx,
									func(k K2, v V2) bool {
										select {
										case <-ctx.Done():
											return false
										case out <- output{kind: data, index: i.index, key: k, value: v}:
											return true
										}
									},
									i.key,
									i.value,
								)
								var kind = complete
								if !ok {
									kind = stop
								}
								select {
								case <-ctx.Done():
									return
								case out <- output{kind: kind, index: i.index}:
								}
							}
						}()
						select {
						case <-ctx.Done():
							return
						case in <- input{nextIndex, k, v}:
							nextIndex++
						}
					}
				} else {
					select {
					case <-ctx.Done():
						return
					case in <- input{nextIndex, k, v}:
						nextIndex++
					}
				}
			}
		}()
		wg.Wait()
	}()

	var wantIndex int
	// 缓冲区，按 index 降序, 因为从尾部取效率最高
	// 长度不会超过 limit, 如果无限制说明调用者不在乎资源占用，所以可以接受。
	var buf []output
	for {
		select {
		case <-ctx.Done():
			return false
		case v, ok := <-out:
			if !ok {
				if len(buf) > 0 && ctx.Err() == nil {
					panic("parallel execution result incomplete") // 防御性断言
				}
				return true
			}
			if v.index == wantIndex {
				// 顺序
				for {
					switch v.kind {
					case data:
						if !yield(v.key, v.value) {
							return false
						}
					case complete:
						wantIndex++
					case stop:
						return false
					default:
						panic(fmt.Errorf("unexpected output kind %q", v.kind))
					}
					// 检查缓冲
					if len(buf) > 0 && buf[len(buf)-1].index == wantIndex {
						v, buf = buf[len(buf)-1], buf[:len(buf)-1]
					} else {
						break
					}
				}
			} else {
				// 乱序, 存入缓冲
				var i, _ = slices.BinarySearchFunc(buf, v, func(el, target output) int {
					if el.index == target.index {
						return 1 // 已有元素在后面
					}

					// 需要降序，所以基于 -index 升序，易懂版本:
					// var key = func(o output) int {
					// 	return -o.index
					// }
					// return key(el) - key(target)

					// 简化结果:
					return target.index - el.index
				})
				buf = slices.Insert(buf, i, v)
			}
		}
	}
}

// ParallelConcatMap2 返回保留顺序的结果.
// 直到前一项产生完成所有结果，才会开始返回下一项的结果。
// limit 限制最大并行数, 非正值代表无限制.
func ParallelConcatMap2[K1, V1, K2, V2 any](
	ctx context.Context,
	limit int,
	seq iter.Seq2[K1, V1],
	yield func(K2, V2) bool,
) func(project func(ctx context.Context, yield func(K2, V2) bool, k K1, v V1) bool) bool {
	return func(project func(ctx context.Context, yield func(K2, V2) bool, k K1, v V1) bool) bool {
		return parallelConcatMap2(
			ctx,
			limit,
			seq,
			yield,
			project,
		)
	}
}

func ParallelConcatMapTo2[T any, K any, V any](
	ctx context.Context,
	limit int,
	seq iter.Seq[T],
	yield func(K, V) bool,
) func(project func(ctx context.Context, yield func(K, V) bool, i T) bool) bool {
	return func(project func(ctx context.Context, yield func(K, V) bool, i T) bool) bool {
		return parallelConcatMap2(
			ctx,
			limit,
			func(yield func(T, struct{}) bool) {
				for i := range seq {
					if !yield(i, struct{}{}) {
						return
					}
				}
			},
			yield,
			func(ctx context.Context, yield func(K, V) bool, k T, v struct{}) bool {
				return project(ctx, yield, k)
			},
		)
	}
}

func ParallelConcatMapFrom2[T any, K any, V any](
	ctx context.Context,
	limit int,
	seq iter.Seq2[K, V],
	yield func(T) bool,
) func(project func(ctx context.Context, yield func(T) bool, k K, v V) bool) bool {
	return func(project func(ctx context.Context, yield func(T) bool, k K, v V) bool) bool {
		return parallelConcatMap2(
			ctx,
			limit,
			seq,
			func(k T, v struct{}) bool {
				return yield(k)
			},
			func(ctx context.Context, yield func(T, struct{}) bool, k K, v V) bool {
				return project(ctx, func(i T) bool {
					return yield(i, struct{}{})
				}, k, v)
			},
		)
	}
}

func ParallelConcatMap[In any, Out any](
	ctx context.Context,
	limit int,
	seq iter.Seq[In],
	yield func(Out) bool,
) func(project func(ctx context.Context, yield func(Out) bool, i In) bool) bool {
	return func(project func(ctx context.Context, yield func(Out) bool, i In) bool) bool {
		return parallelConcatMap2(
			ctx,
			limit,
			func(yield func(In, struct{}) bool) {
				for i := range seq {
					if !yield(i, struct{}{}) {
						return
					}
				}
			},
			func(k Out, _ struct{}) bool {
				return yield(k)
			},
			func(ctx context.Context, yield func(Out, struct{}) bool, k In, v struct{}) bool {
				return project(
					ctx,
					func(o Out) bool { return yield(o, struct{}{}) },
					k,
				)
			},
		)
	}
}
