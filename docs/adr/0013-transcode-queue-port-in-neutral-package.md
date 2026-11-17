# ADR 0013: 转码队列端口移入中立包，结果保留句柄，撤回由 Enqueue ctx 表达

## 状态
Accepted

## 背景

拉取式转码队列的端口族（需求登记、任务认领、等待者、工作项、规格、优先级、分段耗时、结果句柄）原先定义在应用层 `application/image`。结果类型 `File` 是该层特有的抽象，导致唯一的生产 adapter `inmem.TranscodeQueue` 必须反向 import 应用层，仅仅为了命名结果类型；第二个 adapter（持久化实现，或存储字节的测试替身）无法独立演进。

队列的接口宽度接近其实现宽度：同 key 去重、优先级提升、等待者全部撤回则不产出、已认领任务仍可去重等不变量主要以注释形式存在，多锁纪律也只在实现注释里。唯一的真实 adapter 是内存实现，测试里的假队列是为弥补缺失 seam 而存在的 stub。

撤回需求原先由等待者句柄上的 `Cancel()` 显式表达，调用点在 coordinator（`defer waiter.Cancel()`）。这把「需求侧句柄」放到了接口上，而 [ADR 0002](./0002-pull-based-transcode-queue-and-ffmpeg-avif.md) 的语义本来是「abort 即撤回需求」。

## 决定

1. **端口族整体移入 `internal/shared/transcode`**：`Queue` / `Job` / `Waiter` / `Item` / `Spec` / `Prio` / `VariantTiming` / `File` 连同 `ImageFormat` 一起迁入中立包，包名内去掉 `Transcode` 前缀冗余。编排应用与所有 adapter 只依赖该中立包，adapter 不再 import 应用层；不保留旧端口定义或兼容别名。
2. **结果类型保留为可重复打开的句柄**：`File.Open()` 每次返回独立的 `io.ReadSeekCloser`，转码产物与缓存命中产物都通过该抽象交出。不退化为一次性字节值——后者会改变缓存与 HTTP 直出路径的语义，并迫使同 key 的多个等待者各自复制字节。
3. **撤回由 `Enqueue` 的 ctx 生命周期表达**：`Waiter` 不再暴露 `Cancel()`；ctx 取消即撤回该等待者的登记。内存 adapter 把每次登记连同其 ctx 保存，`waitersLocked()` 只统计 ctx 仍活跃的登记；所有活跃登记消失且任务未启动时，`Consume` 淘汰该任务。`Wait` 的 ctx 只结束本次等待，不撤回登记。
4. **计时指针随 Item 构造传入**：编排侧把共享的 `*VariantTiming` 在 `NewItem` 时一并传入，而不是构造后回写私有字段——中立包无法从外部写应用层私有字段，构造时传入也保持「不可变记录」的形状。

## 考虑过的选项

- **把 `File` 换成 `[]byte` 或一次性读取器**：能去掉句柄接口，但缓存命中与转码结果必须各自复制字节，且多个等待者无法安全共享读取位置；改变可观察行为。拒绝。
- **保留 `Cancel()`，同时让 ctx 取消也撤回**：两条撤回路径并存，接口宽度不减，第二个 adapter 仍需实现显式撤回。拒绝。
- **中立包只放 `Queue` / `Job` / `Waiter`，`Spec` 留在应用层**：`Spec` 持有 `ImageFormat`，中立包会因此反向 import 应用层，违反端口方向。拒绝，一并迁入 `ImageFormat`。
- **在 `Enqueue` 中为每个等待者启动 ctx 监听 goroutine 立即递减计数**：撤回更即时，但每个请求引入一个随 ctx 生命周期存在的 goroutine，且已 `Resolve` 的任务仍需额外清理机制。改用「登记保存 ctx、认领时统计活跃数」的惰性判定，语义等价且无 goroutine 开销。

## 后果

- 端口族位置变化要求所有调用方与测试替身同步迁移，无迁移期。
- 撤回的即时性体现为「下次 `Consume` 扫描时」；已认领任务仍跑完入缓存（ADR 0002），撤回只影响未启动任务。
- 优先级提升不会主动唤醒已休眠的消费者；生产 worker 在下一次 `Consume` 迭代（上一个任务完成后）重新扫描并认领，与重构前一致。
- 验收覆盖同 key 去重、优先级提升、等待者全撤回淘汰、已认领任务仍可去重、ctx 撤回与 `File.Open()` 独立性。
- 不重开 ADR 0002 已核实为遵守的条款：singleflight 装饰器删除、缓存归应用层编排且处理器纯转码、magick 处理器无缓存依赖、缓存命中完全不经过队列。
