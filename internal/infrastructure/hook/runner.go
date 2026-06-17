package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"main/internal/domain/device"
	domhook "main/internal/domain/hook"
	"main/internal/domain/image"
	"main/internal/scalar"
	"main/internal/shared"

	"github.com/pelletier/go-toml/v2"
	"go.uber.org/zap"
)

// ImageDispatchTrigger 图片手动触发分发器定义
type ImageDispatchTrigger struct {
	// 未来可扩展
}

// HookConfig 声明式 Hook 配置文件对应的 TOML 数据结构
type HookConfig struct {
	ID          string `toml:"id"`
	Name        string `toml:"name"`
	Description string `toml:"description"`
	Command     string `toml:"command"`
	Triggers    struct {
		PostUpdateImageMetadata *shared.ImageFilters `toml:"post_update_image_metadata"`
		ImageDispatch           *ImageDispatchTrigger           `toml:"image_dispatch"`
	} `toml:"triggers"`
	Env         map[string]string `toml:"env"` // 允许在 TOML 中配置自定义环境变量，键值对都为字符串
	Dir         string            `toml:"-"`   // Hook 配置文件所在的父目录
}

// HookEvent 存储单张图片发生元数据变更的详细属性
type HookEvent struct {
	ID        string
	Path      string
	Rating    int
	Label     string
	Action    string
	OldRating int
	OldLabel  string
	OldAction string
}

// HookExecutionTask 发送给后台串行 Worker 消费的具体进程执行任务
type HookExecutionTask struct {
	HookID      string
	HookName    string
	Command     string
	TriggerName string
	Events      []HookEvent
	Dir         string            // 执行外部命令时的当前工作目录
	Env         map[string]string // 传递给外部脚本的自定义环境变量集合
	ResultChan  chan error        // 用于接收外部脚本执行结果的通道
}

// Debouncer 针对批量 XMP 写入的多 Hook 独立防抖合批组件
type Debouncer struct {
	mu       sync.Mutex
	timer    *time.Timer
	events   map[string][]HookEvent // key: hookID -> events
	duration time.Duration
	callback func(hookID string, events []HookEvent)
}

func NewDebouncer(duration time.Duration, callback func(hookID string, events []HookEvent)) *Debouncer {
	return &Debouncer{
		events:   make(map[string][]HookEvent),
		duration: duration,
		callback: callback,
	}
}

func (d *Debouncer) Add(hookID string, ev HookEvent) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.events[hookID] = append(d.events[hookID], ev)

	if d.timer != nil {
		d.timer.Stop()
	}

	d.timer = time.AfterFunc(d.duration, func() {
		d.mu.Lock()
		eventsCopy := d.events
		d.events = make(map[string][]HookEvent)
		d.timer = nil
		d.mu.Unlock()

		for hID, evs := range eventsCopy {
			if len(evs) > 0 {
				d.callback(hID, evs)
			}
		}
	})
}

// EventBus 本地监听接口
type EventBus interface {
	SubscribeMetadataUpdated(ctx context.Context) iter.Seq2[*shared.MetadataUpdatedEvent, error]
}

var _ domhook.Repository = (*Runner)(nil)
var _ domhook.Runner = (*Runner)(nil)

// Runner 外部钩子管理器（应用/基础设施适配器服务）
type Runner struct {
	rootDir       string
	hooksDir      string
	logger        *zap.Logger
	ebus          EventBus
	graphqlURL    string
	tokenSource   device.TokenSource
	filterBuilder *image.FilterBuilder
	ch            chan HookExecutionTask
	debouncer     *Debouncer
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

func NewRunner(rootDir string, hooksDir string, logger *zap.Logger, ebus EventBus, graphqlURL string, tokenSource device.TokenSource, filterBuilder *image.FilterBuilder) *Runner {
	ctx, cancel := context.WithCancel(context.Background())
	r := &Runner{
		rootDir:       rootDir,
		hooksDir:      hooksDir,
		logger:        logger,
		ebus:          ebus,
		graphqlURL:    graphqlURL,
		tokenSource:   tokenSource,
		filterBuilder: filterBuilder,
		ch:            make(chan HookExecutionTask, 1024),
		ctx:           ctx,
		cancel:        cancel,
	}

	r.debouncer = NewDebouncer(100*time.Millisecond, r.onDebounceTrigger)

	r.wg.Add(2)
	go func() {
		defer r.wg.Done()
		r.runListener(ctx)
	}()
	go func() {
		defer r.wg.Done()
		r.runWorker(ctx)
	}()

	return r
}

func (r *Runner) Close() {
	r.cancel()
	close(r.ch)
	r.wg.Wait()
}

func (r *Runner) List(ctx context.Context) ([]*domhook.Hook, error) {
	hooks, err := r.LoadHooks()
	if err != nil {
		return nil, err
	}
	var res []*domhook.Hook
	for _, h := range hooks {
		res = append(res, domhook.FromRepository(h.ID, h.Name, h.Description, h.Triggers.ImageDispatch != nil))
	}
	return res, nil
}

func (r *Runner) Trigger(ctx context.Context, ids []string, paths []string, hookID scalar.ID, triggerName string) error {
	hooks, err := r.LoadHooks()
	if err != nil {
		return err
	}

	var targetHook *HookConfig
	for _, h := range hooks {
		domH := domhook.FromRepository(h.ID, h.Name, h.Description, h.Triggers.ImageDispatch != nil)
		if domH.ID() == hookID {
			targetHook = &h
			break
		}
	}

	if targetHook == nil {
		return fmt.Errorf("hook %s not found", hookID.String())
	}

	if triggerName == "image_dispatch" && targetHook.Triggers.ImageDispatch == nil {
		return fmt.Errorf("hook %s does not allow manual dispatch", hookID.String())
	}

	var events []HookEvent
	for i, id := range ids {
		var path string
		if i < len(paths) {
			path = paths[i]
		}

		events = append(events, HookEvent{
			ID:        id,
			Path:      path,
			Rating:    0,
			Label:     "",
			Action:    "",
			OldRating: 0,
			OldLabel:  "",
			OldAction: "",
		})
	}

	resChan := make(chan error, 1)
	r.ch <- HookExecutionTask{
		HookID:      targetHook.ID,
		HookName:    targetHook.Name,
		Command:     targetHook.Command,
		TriggerName: triggerName,
		Events:      events,
		Dir:         targetHook.Dir,
		Env:         targetHook.Env,
		ResultChan:  resChan,
	}

	// 同步等待执行结果或 context 被取消
	select {
	case err := <-resChan:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// runListener 异步监听 EventBus 发来的元数据修改事件
func (r *Runner) runListener(ctx context.Context) {
	for event, err := range r.ebus.SubscribeMetadataUpdated(ctx) {
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}

		r.handleMetadataUpdated(event)
	}
}

func (r *Runner) handleMetadataUpdated(event *shared.MetadataUpdatedEvent) {
	hooks, err := r.LoadHooks()
	if err != nil {
		r.logger.Error("failed to load hooks during metadata updated event", zap.Error(err))
		return
	}

	for _, h := range hooks {
		trigger := h.Triggers.PostUpdateImageMetadata
		if trigger == nil {
			continue
		}

		// 使用领域层的 Filter 匹配机制进行过滤
		filterFunc := r.filterBuilder.Build(*trigger)
		if !filterFunc(eventWrapper{event}) {
			continue
		}

		// 加入防抖
		r.debouncer.Add(h.ID, HookEvent{
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

type eventWrapper struct {
	*shared.MetadataUpdatedEvent
}

func (w eventWrapper) ID() scalar.ID {
	return w.MetadataUpdatedEvent.ID
}

func (w eventWrapper) DirectoryID() scalar.ID {
	return scalar.ToID("")
}

func (w eventWrapper) Rating() int {
	return w.MetadataUpdatedEvent.Rating
}

func (w eventWrapper) Label() string {
	return w.MetadataUpdatedEvent.Label
}

func (w eventWrapper) Filename() string {
	return filepath.Base(w.MetadataUpdatedEvent.Path)
}

func (r *Runner) LoadHooks() ([]HookConfig, error) {
	if r.hooksDir == "" {
		return nil, nil
	}
	var configs []HookConfig

	entries, err := os.ReadDir(r.hooksDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if entry.IsDir() || ext != ".toml" {
			continue
		}

		path := filepath.Join(r.hooksDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			r.logger.Warn("failed to read hook config file", zap.String("path", path), zap.Error(err))
			continue
		}

		var cfg HookConfig
		if err := toml.Unmarshal(data, &cfg); err != nil {
			r.logger.Warn("failed to parse hook config toml", zap.String("path", path), zap.Error(err))
			continue
		}

		cfg.Dir = filepath.Dir(path) // 记录 Hook 配置文件所在目录

		if cfg.ID == "" {
			cfg.ID = strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		}
		if cfg.Command == "" {
			r.logger.Warn("hook command is empty, skip loading", zap.String("id", cfg.ID))
			continue
		}

		configs = append(configs, cfg)
	}

	return configs, nil
}

func (r *Runner) onDebounceTrigger(hookID string, events []HookEvent) {
	hooks, err := r.LoadHooks()
	if err != nil {
		r.logger.Error("failed to load hooks on debounce callback", zap.Error(err))
		return
	}

	var targetHook *HookConfig
	for _, h := range hooks {
		if h.ID == hookID {
			targetHook = &h
			break
		}
	}

	if targetHook == nil {
		return
	}

	r.ch <- HookExecutionTask{
		HookID:      targetHook.ID,
		HookName:    targetHook.Name,
		Command:     targetHook.Command,
		TriggerName: "post_update_image_metadata",
		Events:      events,
		Dir:         targetHook.Dir,
		Env:         targetHook.Env,
	}
}

func (r *Runner) runWorker(ctx context.Context) {
	for {
		select {
		case task, ok := <-r.ch:
			if !ok {
				return
			}
			r.executeHook(ctx, task)
		case <-ctx.Done():
			return
		}
	}
}

func (r *Runner) executeHook(ctx context.Context, task HookExecutionTask) {
	var ids []string
	var paths []string

	var rating, oldRating int
	var label, oldLabel, action, oldAction string

	for _, ev := range task.Events {
		ids = append(ids, ev.ID)
		paths = append(paths, ev.Path)
	}

	idsJSON, _ := json.Marshal(ids)
	pathsJSON, _ := json.Marshal(paths)

	if len(task.Events) == 1 {
		rating = task.Events[0].Rating
		label = task.Events[0].Label
		action = task.Events[0].Action
		oldRating = task.Events[0].OldRating
		oldLabel = task.Events[0].OldLabel
		oldAction = task.Events[0].OldAction
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd.exe", "/c", task.Command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", task.Command)
	}

	cmd.Dir = task.Dir // 将脚本的工作目录设置为 Hook 配置文件所在的目录

	env := append(os.Environ(),
		"IMAGE_FUNNEL_HOOK_ID="+task.HookID,
		"IMAGE_FUNNEL_HOOK_NAME="+task.HookName,
		"IMAGE_FUNNEL_TRIGGER="+task.TriggerName,
		"IMAGE_FUNNEL_IMAGE_IDS="+string(idsJSON),
		"IMAGE_FUNNEL_IMAGE_PATHS="+string(pathsJSON),
		"IMAGE_FUNNEL_IMAGE_RATING="+fmt.Sprintf("%d", rating),
		"IMAGE_FUNNEL_IMAGE_LABEL="+label,
		"IMAGE_FUNNEL_IMAGE_ACTION="+action,
		"IMAGE_FUNNEL_IMAGE_OLD_RATING="+fmt.Sprintf("%d", oldRating),
		"IMAGE_FUNNEL_IMAGE_OLD_LABEL="+oldLabel,
		"IMAGE_FUNNEL_IMAGE_OLD_ACTION="+oldAction,
	)

	// 注入来自 TOML 配置文件中 [env] 节的自定义环境变量
	for k, v := range task.Env {
		env = append(env, k+"="+v)
	}

	if r.graphqlURL != "" {
		env = append(env, "IMAGE_FUNNEL_GRAPHQL_URL="+r.graphqlURL)
	}

	if r.tokenSource != nil {
		// 签发临时的 JWT 令牌供外部脚本高频访问 GraphQL API 鉴权使用
		tok, err := r.tokenSource.NewAccessToken(ctx, scalar.ToID("hook-runner"))
		if err == nil {
			env = append(env, "IMAGE_FUNNEL_TOKEN="+tok.String())
		} else {
			r.logger.Warn("failed to generate temporary token for hook", zap.Error(err))
		}
	}

	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	r.logger.Info("will execute external hook command",
		zap.String("hook_id", task.HookID),
		zap.String("trigger", task.TriggerName),
		zap.Int("batch_size", len(task.Events)),
	)

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	if err != nil {
		r.logger.Error("external hook command failed",
			zap.String("hook_id", task.HookID),
			zap.Duration("duration", duration),
			zap.Error(err),
			zap.String("stdout", stdout.String()),
			zap.String("stderr", stderr.String()),
		)
		if task.ResultChan != nil {
			stderrStr := strings.TrimSpace(stderr.String())
			if stderrStr != "" {
				task.ResultChan <- fmt.Errorf("hook script failed: %w, stderr: %s", err, stderrStr)
			} else {
				task.ResultChan <- fmt.Errorf("hook script failed: %w", err)
			}
		}
		return
	}

	r.logger.Info("external hook command completed",
		zap.String("hook_id", task.HookID),
		zap.Duration("duration", duration),
		zap.String("stdout", stdout.String()),
	)
	if task.ResultChan != nil {
		task.ResultChan <- nil
	}
}


