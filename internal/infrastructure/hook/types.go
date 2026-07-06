package hook

import (
	"sync"
	"time"
)

// hookEvent 存储单张图片发生元数据变更的详细属性
type hookEvent struct {
	ID        string
	Path      string
	Rating    int
	Label     string
	Action    string
	OldRating int
	OldLabel  string
	OldAction string
}

// hookExecutionResult 外部脚本执行结果
type hookExecutionResult struct {
	Error  error  // 脚本执行错误，nil 表示成功
	Action string // 脚本通过 IMAGE_FUNNEL_ACTION 文件指定的操作，空字符串表示未覆盖
	Stdout string // 脚本标准输出
	Stderr string // 脚本标准错误输出
}

// hookExecutionTask 发送给后台串行 Worker 消费的具体进程执行任务
type hookExecutionTask struct {
	HookID       string
	HookName     string
	Command      string   // 基础命令（如 "uv run comfyui.py remove"）
	ExtraArgs    []string // 额外参数，原样传递给命令
	TriggerName  string
	Events       []hookEvent
	Dir          string                   // 执行外部命令时的当前工作目录
	Env          map[string]string        // 传递给外部脚本的自定义环境变量集合
	NotePath     string                   // 笔记文件的相对路径
	DirectoryID  string                   // 会话或笔记所在的目录ID
	DirectoryRel string                   // 目录的相对路径
	resultChan   chan hookExecutionResult // 用于接收外部脚本执行结果的通道
	RunID        string                   // 指令运行 ID 注入环境变量
}

// debouncer 针对批量 XMP 写入的多 Hook 独立防抖合批组件
type debouncer struct {
	mu       sync.Mutex
	timer    *time.Timer
	events   map[string][]hookEvent // key: hookID -> events
	duration time.Duration
	callback func(hookID string, events []hookEvent)
}

func newDebouncer(duration time.Duration, callback func(hookID string, events []hookEvent)) *debouncer {
	return &debouncer{
		events:   make(map[string][]hookEvent),
		duration: duration,
		callback: callback,
	}
}

func (d *debouncer) Add(hookID string, ev hookEvent) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.events[hookID] = append(d.events[hookID], ev)

	if d.timer != nil {
		d.timer.Stop()
	}

	d.timer = time.AfterFunc(d.duration, func() {
		d.mu.Lock()
		eventsCopy := d.events
		d.events = make(map[string][]hookEvent)
		d.timer = nil
		d.mu.Unlock()

		for hID, evs := range eventsCopy {
			if len(evs) > 0 {
				d.callback(hID, evs)
			}
		}
	})
}

type writeIgnoreItem struct {
	contentHash uint64
	expireTime  time.Time
}

type taskPhase int

const (
	phaseBefore3 taskPhase = iota
	phaseAfter3
)

type activeTask struct {
	phase            taskPhase
	paths            map[string]struct{}
	failedDirectives map[string]bool
}
