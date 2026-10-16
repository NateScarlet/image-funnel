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

// debouncer 针对批量 XMP 写入的多 Hook 独立防抖合批组件
type debouncer struct {
	mu       sync.Mutex
	timer    *time.Timer
	events   map[string][]hookEvent // key: hookID -> events
	keys     []string               // 插入顺序，用于按序回调
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

	if _, exists := d.events[hookID]; !exists {
		d.keys = append(d.keys, hookID)
	}
	d.events[hookID] = append(d.events[hookID], ev)

	if d.timer != nil {
		d.timer.Stop()
	}

	d.timer = time.AfterFunc(d.duration, func() {
		d.mu.Lock()
		eventsCopy := d.events
		keysCopy := d.keys
		d.events = make(map[string][]hookEvent)
		d.keys = nil
		d.timer = nil
		d.mu.Unlock()

		for _, hID := range keysCopy {
			if evs, ok := eventsCopy[hID]; ok && len(evs) > 0 {
				d.callback(hID, evs)
			}
		}
	})
}
