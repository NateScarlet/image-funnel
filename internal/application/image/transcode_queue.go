package image

import (
	"context"
	"fmt"
	"iter"
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

// Spec 图片变体的转码规格：源图身份之外的变体参数三元组
type Spec struct {
	width   int
	quality int
	format  ImageFormat
}

func NewSpec(width, quality int, format ImageFormat) (Spec, error) {
	// 可信边界校验：宽度/质量非负、格式合法；内部数据创建后即合法
	if width < 0 {
		return Spec{}, fmt.Errorf("width must be non-negative, got %d", width)
	}
	if quality < 0 {
		return Spec{}, fmt.Errorf("quality must be non-negative, got %d", quality)
	}
	if format != ImageFormatWebP && format != ImageFormatAVIF {
		return Spec{}, fmt.Errorf("unsupported format: %d", format)
	}
	return Spec{width: width, quality: quality, format: format}, nil
}

func (s Spec) Width() int   { return s.width }
func (s Spec) Quality() int { return s.quality }
func (s Spec) Format() ImageFormat {
	return s.format
}

// TranscodeItem 一次变体转码需求的不可变记录。只携带命令信息（去重键、源路径与规格），
// 不携带文件对象或执行逻辑——执行由 worker 侧的 Processor 在拉取时完成，
// 源文件状态（大小/修改时间）在执行时才解析，队列中等待不会导致状态过期
type TranscodeItem struct {
	key      string
	srcPath  string
	spec     Spec
	priority Prio
}

// NewTranscodeItem 创建变体转码需求。key 是变体去重键（源路径+规格派生），
// 由调用方基于执行时可复算的输入生成
func NewTranscodeItem(key, srcPath string, spec Spec, priority Prio) (TranscodeItem, error) {
	if key == "" {
		return TranscodeItem{}, fmt.Errorf("transcode item key must not be empty")
	}
	if srcPath == "" {
		return TranscodeItem{}, fmt.Errorf("transcode item src path must not be empty")
	}
	if priority != PrioLow && priority != PrioHigh {
		return TranscodeItem{}, fmt.Errorf("unsupported priority: %d", priority)
	}
	return TranscodeItem{key: key, srcPath: srcPath, spec: spec, priority: priority}, nil
}

func (i TranscodeItem) Key() string      { return i.key }
func (i TranscodeItem) SrcPath() string  { return i.srcPath }
func (i TranscodeItem) Spec() Spec       { return i.spec }
func (i TranscodeItem) Priority() Prio   { return i.priority }

// TranscodeWaiter 请求侧句柄：等待一次转码完成或显式撤回需求。
// 多个等待者共享同一执行（同 key 去重）
type TranscodeWaiter interface {
	// Wait 纯观察：阻塞至该 key 的执行完成。ctx 取消仅结束本次等待，
	// 不撤回需求登记（撤回必须显式调用 Cancel）
	Wait(ctx context.Context) (File, error)
	// Cancel 显式撤回本等待者的需求登记。所有等待者都撤回且任务未启动时，
	// 队列不会再将该任务交给 worker
	Cancel()
}

// TranscodeJob worker 侧句柄。队列保证交给 worker 的都是仍有等待者的任务
type TranscodeJob interface {
	Item() TranscodeItem
	// Resolve 结束执行：结果或错误广播给该 key 的所有等待者
	Resolve(file File, err error)
}

// TranscodeQueue 拉取式转码队列端口：请求侧登记需求，worker 侧按优先级阈值拉取执行
type TranscodeQueue interface {
	// Enqueue 登记一个变体转码需求。同 key 的并发请求返回指向同一执行的等待者；
	// 已有低优先级等待者的 key 再收到高优先级等待者时，该任务优先级提升
	Enqueue(ctx context.Context, item TranscodeItem) (TranscodeWaiter, error)
	// Consume 迭代产出等待执行的任务，仅返回优先级 ≥ minPriority 的任务，
	// 且等待者已全部撤回的未启动任务不会产出。循环持续直到 ctx 取消
	Consume(ctx context.Context, minPriority Prio) iter.Seq2[TranscodeJob, error]
}
