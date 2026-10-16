package hook

import (
	"context"
	"slices"
	"time"

	"main/internal/shared"

	"go.uber.org/zap"
)

// imageMetadataTrigger 元数据更新触发路径（post_update_image_metadata）：
// 按配置评估事件是否命中某个钩子的筛选条件，命中的事件按钩子分组防抖合批后串行执行。
type imageMetadataTrigger struct {
	bgCtx     context.Context
	config    configSource
	exec      *hookExecutor
	notes     *noteContext
	debouncer *debouncer
	logger    *zap.Logger
}

func newImageMetadataTrigger(
	bgCtx context.Context,
	config configSource,
	exec *hookExecutor,
	notes *noteContext,
	debounceDuration time.Duration,
	logger *zap.Logger,
) *imageMetadataTrigger {
	t := &imageMetadataTrigger{
		bgCtx:  bgCtx,
		config: config,
		exec:   exec,
		notes:  notes,
		logger: logger,
	}
	t.debouncer = newDebouncer(debounceDuration, t.onDebounced)
	return t
}

// Handle 评估事件是否匹配某个 post_update_image_metadata 钩子，匹配则按钩子加入防抖队列
func (t *imageMetadataTrigger) Handle(event *shared.MetadataUpdatedEvent) {
	hooks, err := t.config.Load()
	if err != nil {
		t.logger.Error("failed to load hooks during metadata updated event", zap.Error(err))
		return
	}

	// 按 (order, Filename) 排序，确保防抖回调按相同顺序投递任务
	slices.SortStableFunc(hooks, sortByOrderAndFilename)

	for _, h := range hooks {
		trigger := h.On.PostUpdateImageMetadata
		if trigger == nil {
			continue
		}

		// 直接评估事件是否匹配钩子的过滤规则
		if !trigger.match(event) {
			continue
		}

		// 加入防抖
		t.debouncer.Add(h.ID, hookEvent{
			ID:        event.ID.String(),
			Path:      event.Path,
			Rating:    event.Rating,
			Label:     event.Label,
			Action:    event.Action,
			OldRating: event.OldRating,
			OldLabel:  event.OldLabel,
			OldAction: event.OldAction,
		})
	}
}

// onDebounced 防抖窗口结束后按配置重新解析该钩子并投递执行。
// 重新解析而非沿用入队时的配置，使合批窗口内的配置改动即时生效。
func (t *imageMetadataTrigger) onDebounced(hookID string, events []hookEvent) {
	hooks, err := t.config.Load()
	if err != nil {
		t.logger.Error("failed to load hooks on debounce callback", zap.Error(err))
		return
	}

	for _, h := range hooks {
		if h.ID != hookID {
			continue
		}
		t.dispatch(h, events)
		return
	}
}

func (t *imageMetadataTrigger) dispatch(h hookConfig, events []hookEvent) {
	// 从第一个事件中推导目录信息，触发必然涉及图片，目录信息总是可推导的
	dir, err := t.notes.DirOf(t.bgCtx, events[0].Path)
	if err != nil {
		t.logger.Error("failed to resolve directory for hook event, skipping",
			zap.String("hook_id", h.ID),
			zap.String("trigger", "post_update_image_metadata"),
			zap.String("event_path", events[0].Path),
			zap.Error(err),
		)
		return
	}

	t.exec.Enqueue(newHookTask(h, "post_update_image_metadata", events, "", dir))
}
