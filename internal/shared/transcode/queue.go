package transcode

import (
	"context"
	"fmt"
	"iter"
	"time"
)

// Prio 转码任务的优先级。数值越大优先级越高，Consume 的阈值语义（返回 ≥ minPriority）
// 依赖该单调性；新增级别时必须插在两端之间而非改变端点含义
type Prio int

const (
	// PrioLow 预载/预热需求，可被随时撤回
	PrioLow Prio = iota
	// PrioHigh 用户当前查看需求
	PrioHigh
)

// VariantTiming 一次变体取回的分段耗时，供图片响应的 Server-Timing 使用。
// 队列等待由队列在认领任务时写入，编码耗时由 worker 在 Process 前后写入；
// 指针随 Item 在等待者与执行者之间共享
type VariantTiming struct {
	EnqueuedAt     time.Time
	QueueDuration  time.Duration
	EncodeDuration time.Duration
	CacheHit       bool
}

// Item 一次变体转码需求的不可变记录。只携带命令信息（去重键、源路径与规格），
// 不携带文件对象或执行逻辑——执行由 worker 侧的 Processor 在拉取时完成，
// 源文件状态（大小/修改时间）在执行时才解析，队列中等待不会导致状态过期
type Item struct {
	key      string
	srcPath  string
	spec     Spec
	priority Prio
	timing   *VariantTiming
}

// NewItem 创建变体转码需求。key 是变体去重键（源路径+规格派生），
// 由调用方基于执行时可复算的输入生成；timing 可为 nil，由调用方与执行者共享
func NewItem(key, srcPath string, spec Spec, priority Prio, timing *VariantTiming) (Item, error) {
	if key == "" {
		return Item{}, fmt.Errorf("transcode item key must not be empty")
	}
	if srcPath == "" {
		return Item{}, fmt.Errorf("transcode item src path must not be empty")
	}
	if priority != PrioLow && priority != PrioHigh {
		return Item{}, fmt.Errorf("unsupported priority: %d", priority)
	}
	return Item{key: key, srcPath: srcPath, spec: spec, priority: priority, timing: timing}, nil
}

func (i Item) Key() string            { return i.key }
func (i Item) SrcPath() string        { return i.srcPath }
func (i Item) Spec() Spec             { return i.spec }
func (i Item) Priority() Prio         { return i.priority }
func (i Item) Timing() *VariantTiming { return i.timing }

// Waiter 请求侧句柄：等待一次转码完成，多个等待者共享同一执行（同 key 去重）。
// 需求登记的生命周期由 Enqueue 的 ctx 表达：ctx 取消即撤回本等待者的登记。
type Waiter interface {
	// Wait 阻塞至该 key 的执行完成或本次等待的 ctx 结束；
	// 本次 ctx 仅结束等待，不撤回需求登记（撤回由 Enqueue 的 ctx 负责）
	Wait(ctx context.Context) (File, error)
	// Timing 返回本次取回的分段耗时；缓存命中时 CacheHit 为真
	Timing() *VariantTiming
}

// Job worker 侧句柄。队列保证交给 worker 的都是仍有等待者的任务
type Job interface {
	Item() Item
	// Resolve 结束执行：结果或错误广播给该 key 的所有等待者
	Resolve(file File, err error)
}

// Queue 拉取式转码队列端口：请求侧登记需求，worker 侧按优先级阈值拉取执行。
// 同 key 去重、优先级提升、等待者全部撤回则不产出未启动任务等不变量由实现保证。
type Queue interface {
	// Enqueue 登记一个变体转码需求。同 key 的并发请求返回指向同一执行的等待者；
	// 已有低优先级等待者的 key 再收到高优先级等待者时，该任务优先级提升。
	// ctx 取消即撤回本次登记：所有等待者都撤回且任务未启动时，队列不再产出该任务
	Enqueue(ctx context.Context, item Item) (Waiter, error)
	// Consume 迭代产出等待执行的任务，仅返回优先级 ≥ minPriority 的任务，
	// 且等待者已全部撤回的未启动任务不会产出。循环持续直到 ctx 取消
	Consume(ctx context.Context, minPriority Prio) iter.Seq2[Job, error]
}
