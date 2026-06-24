package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"maps"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"main/internal/apperror"
	"main/internal/domain/device"
	domdir "main/internal/domain/directory"
	domhook "main/internal/domain/hook"
	domimage "main/internal/domain/image"
	"main/internal/scalar"
	"main/internal/shared"

	"github.com/cespare/xxhash/v2"
	"github.com/pelletier/go-toml/v2"
	"go.uber.org/zap"
)

var fastCheckReg = regexp.MustCompile(`(?m)^[ \t]*/[a-zA-Z0-9_-]+`)

var directiveReg = regexp.MustCompile(`(?m)^[ \t]*/([a-zA-Z0-9_-]+)(?:\s+([^\r\n]*))?\r?\n?`)

// ImageDispatchTrigger 图片手动触发分发器定义
type ImageDispatchTrigger struct {
}

// DirectiveConfig 钩子提供的笔记指令配置
type DirectiveConfig struct {
	Name            string `toml:"name"`
	Usage           string `toml:"usage"`
	OnSuccessAction string `toml:"on_success_action"`
	OnFailAction    string `toml:"on_fail_action"`
}

// HookConfig 声明式 Hook 配置文件对应的 TOML 数据结构
type HookConfig struct {
	ID          string           `toml:"id"`
	Name        string           `toml:"name"`
	Description string           `toml:"description"`
	Command     string           `toml:"command"`
	Directive   *DirectiveConfig `toml:"directive"` // 笔记指令定义
	On          struct {
		PostUpdateImageMetadata *Filters              `toml:"post_update_image_metadata"`
		ImageDispatch           *ImageDispatchTrigger `toml:"image_dispatch"`
		PostUpdateNote          *struct {
			IgnoreDirective bool `toml:"ignore_directive"`
		} `toml:"post_update_note"`
		PostCommitSession *struct {
			NoteScan *struct {
				IgnoreDirective bool `toml:"ignore_directive"`
			} `toml:"note_scan"`
		} `toml:"post_commit_session"`
		NoteDispatch *struct {
		} `toml:"note_dispatch"`
	} `toml:"on"`
	Env map[string]string `toml:"env"` // 允许在 TOML 中配置自定义环境变量，键值对都为字符串
	Dir string            `toml:"-"`   // Hook 配置文件所在的父目录
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

// HookExecutionResult 外部脚本执行结果
type HookExecutionResult struct {
	Error  error  // 脚本执行错误，nil 表示成功
	Action string // 脚本通过 IMAGE_FUNNEL_ACTION 文件指定的操作，空字符串表示未覆盖
}

// HookExecutionTask 发送给后台串行 Worker 消费的具体进程执行任务
type HookExecutionTask struct {
	HookID       string
	HookName     string
	Command      string   // 基础命令（如 "python comfyui.py remove"）
	ExtraArgs    []string // 额外参数，原样传递给命令
	TriggerName  string
	Events       []HookEvent
	Dir          string                   // 执行外部命令时的当前工作目录
	Env          map[string]string        // 传递给外部脚本的自定义环境变量集合
	NotePath     string                   // 笔记文件的相对路径
	DirectoryID  string                   // 会话或笔记所在的目录ID
	DirectoryRel string                   // 目录的相对路径
	ResultChan   chan HookExecutionResult // 用于接收外部脚本执行结果的通道
	RunID        string                   // 指令运行 ID 注入环境变量
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

// ImageRepository 本地图像仓库接口
type ImageRepository interface {
	Get(ctx context.Context, relPath string) (*domimage.Image, error)
	Find(ctx context.Context, relPath string) iter.Seq2[*domimage.Image, error]
}

// DirectoryService 本地目录服务接口
type DirectoryService interface {
	GetDirectory(ctx context.Context, id scalar.ID) (*domdir.Directory, error)
}

// EventBus 本地监听接口
type EventBus interface {
	SubscribeMetadataUpdated(ctx context.Context) iter.Seq2[*shared.MetadataUpdatedEvent, error]
	SubscribeFileChanged(ctx context.Context) iter.Seq2[*shared.FileChangedEvent, error]
}

var _ domhook.Repository = (*Runner)(nil)
var _ domhook.Runner = (*Runner)(nil)

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

// Runner 外部钩子管理器（应用/基础设施适配器服务）
type Runner struct {
	rootDir     string
	hooksDir    string
	logger      *zap.Logger
	ebus        EventBus
	graphqlURL  string
	tokenSource device.TokenSource
	imgRepo     ImageRepository
	dirSvc      DirectoryService
	dirRepo     domdir.Repository
	ch          chan HookExecutionTask
	debouncer   *Debouncer
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	muIgnore    sync.Mutex
	writeIgnore map[string]writeIgnoreItem
	muTasks     sync.Mutex
	activeTasks map[string]*activeTask
}

func NewRunner(
	rootDir string,
	hooksDir string,
	logger *zap.Logger,
	ebus EventBus,
	graphqlURL string,
	tokenSource device.TokenSource,
	imgRepo ImageRepository,
	dirSvc DirectoryService,
	dirRepo domdir.Repository,
) *Runner {
	ctx, cancel := context.WithCancel(context.Background())
	r := &Runner{
		rootDir:     rootDir,
		hooksDir:    hooksDir,
		logger:      logger,
		ebus:        ebus,
		graphqlURL:  graphqlURL,
		tokenSource: tokenSource,
		imgRepo:     imgRepo,
		dirSvc:      dirSvc,
		dirRepo:     dirRepo,
		ch:          make(chan HookExecutionTask, 1024),
		ctx:         ctx,
		cancel:      cancel,
		writeIgnore: make(map[string]writeIgnoreItem),
		activeTasks: make(map[string]*activeTask),
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
		hasPostUpdateNote := h.On.PostUpdateNote != nil
		hasPostCommitSessionNoteScan := h.On.PostCommitSession != nil && h.On.PostCommitSession.NoteScan != nil
		res = append(res, domhook.FromRepository(
			h.ID,
			h.Name,
			h.Description,
			h.On.ImageDispatch != nil,
			h.On.NoteDispatch != nil,
			toDomainDirective(h.Directive),
			hasPostUpdateNote,
			hasPostCommitSessionNoteScan,
		))
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
		domH := domhook.FromRepository(h.ID, h.Name, h.Description, h.On.ImageDispatch != nil, h.On.NoteDispatch != nil, nil, false, false)
		if domH.ID() == hookID {
			targetHook = &h
			break
		}
	}

	if targetHook == nil {
		return fmt.Errorf("hook %s not found", hookID.String())
	}

	if triggerName == "image_dispatch" && targetHook.On.ImageDispatch == nil {
		return fmt.Errorf("hook %s does not allow manual dispatch", hookID.String())
	}
	if triggerName == "note_dispatch" && targetHook.On.NoteDispatch == nil {
		return fmt.Errorf("hook %s does not allow note dispatch", hookID.String())
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

	_, err = r.executeHookSync(*targetHook, triggerName, events, nil, "", "", "", "")
	return err
}

// TriggerForNote 手动派发笔记触发的外部钩子任务
func (r *Runner) TriggerForNote(ctx context.Context, noteRelPath string, hookID scalar.ID) error {
	r.logger.Debug("TriggerForNote start", zap.String("noteRelPath", noteRelPath), zap.String("hookID", hookID.String()))
	hooks, err := r.LoadHooks()
	if err != nil {
		return err
	}

	var targetHook *HookConfig
	for _, h := range hooks {
		domH := domhook.FromRepository(h.ID, h.Name, h.Description, h.On.ImageDispatch != nil, h.On.NoteDispatch != nil, nil, false, false)
		r.logger.Debug("TriggerForNote comparing hook", zap.String("h.ID", h.ID), zap.String("domH.ID", domH.ID().String()))
		if domH.ID() == hookID {
			targetHook = &h
			break
		}
	}

	if targetHook == nil {
		return fmt.Errorf("hook %s not found", hookID.String())
	}

	if targetHook.On.NoteDispatch == nil {
		return fmt.Errorf("hook %s does not allow note dispatch", hookID.String())
	}

	// 寻找配套的图片
	events, err := r.findAssociatedImageEvents(ctx, noteRelPath)
	if err != nil {
		return fmt.Errorf("failed to get associated image for note dispatch: %w", err)
	}

	dirRelPath := filepath.Dir(noteRelPath)
	if dirRelPath == "." {
		dirRelPath = ""
	}

	// 依照领域仓库通过相对路径加载目录实体以提取其 ID，避免外部自行编码
	dir, err := r.dirRepo.Get(ctx, dirRelPath)
	if err != nil {
		return fmt.Errorf("failed to get directory for note dispatch: %w", err)
	}
	dirID := dir.ID()

	if targetHook.Directive != nil && targetHook.Directive.Name != "" {
		r.logger.Debug("TriggerForNote directive matches, will execute directives", zap.String("directiveName", targetHook.Directive.Name))
		noteAbsPath := filepath.Join(r.rootDir, noteRelPath)
		contentBytes, err := os.ReadFile(noteAbsPath)
		if err != nil {
			return fmt.Errorf("failed to read note file for dispatch: %w", err)
		}
		content := string(contentBytes)

		executed, err := r.executeNoteDirectives(ctx, dirID, dirRelPath, noteRelPath, content, "note_dispatch", hookID)
		r.logger.Debug("TriggerForNote executeNoteDirectives finished", zap.Bool("executed", executed), zap.Error(err))
		if err != nil {
			return err
		}

		if !executed {
			r.logger.Debug("TriggerForNote not executed by directives, fallback to executeHookSync", zap.String("hookID", targetHook.ID))
			_, err = r.executeHookSync(*targetHook, "note_dispatch", events, nil, noteRelPath, dirID.String(), dirRelPath, "")
			return err
		}
		return nil
	}

	r.logger.Debug("TriggerForNote no directive defined, executing hook directly", zap.String("hookID", targetHook.ID))
	_, err = r.executeHookSync(*targetHook, "note_dispatch", events, nil, noteRelPath, dirID.String(), dirRelPath, "")
	return err
}

// runListener 异步监听 EventBus 发来的元数据修改事件以及文件变更事件
func (r *Runner) runListener(ctx context.Context) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		for event, err := range r.ebus.SubscribeFileChanged(ctx) {
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				continue
			}
			r.handleFileChanged(event)
		}
	}()

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

func (r *Runner) handleFileChanged(event *shared.FileChangedEvent) {
	if event.Action != shared.FileActionCreate && event.Action != shared.FileActionWrite {
		return
	}
	if strings.ToLower(filepath.Ext(event.RelPath)) != ".md" {
		return
	}

	noteAbsPath := filepath.Join(r.rootDir, event.RelPath)
	contentBytes, err := os.ReadFile(noteAbsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		r.logger.Error("failed to read note file for directive processing", zap.String("path", event.RelPath), zap.Error(err))
		return
	}

	content := string(contentBytes)

	// 0. 检查内容哈希防重入列表，若是自身写入触发的事件则直接忽略
	if r.shouldIgnoreEvent(noteAbsPath, contentBytes) {
		r.logger.Debug("ignoring change event as it was triggered by our own write", zap.String("path", event.RelPath))
		return
	}

	dirRelPath := filepath.Dir(event.RelPath)
	if dirRelPath == "." {
		dirRelPath = ""
	}

	// 1. 处理包含指令的钩子：执行指令并回写文本
	// 磁盘写回职责已完全收拢至 processSingleNote 内部，外层不再负责二次写入
	_, err = r.executeNoteDirectives(r.ctx, event.DirectoryID, dirRelPath, event.RelPath, content, "post_update_note", scalar.ID{})
	if err != nil {
		r.logger.Error("failed to process note directives for file change", zap.String("path", event.RelPath), zap.Error(err))
		return
	}

	// 2. 触发无指令要求的笔记修改钩子 (h.Directive == nil 或 requires_directive = false)
	hooks, err := r.LoadHooks()
	if err != nil {
		return
	}

	var noDirectiveHooks []HookConfig
	for _, h := range hooks {
		if h.On.PostUpdateNote == nil {
			continue
		}
		if h.Directive == nil || h.On.PostUpdateNote.IgnoreDirective {
			noDirectiveHooks = append(noDirectiveHooks, h)
		}
	}

	if len(noDirectiveHooks) > 0 {
		evs, err := r.findAssociatedImageEvents(r.ctx, event.RelPath)
		if err != nil {
			r.logger.Error("failed to get associated image for note update hook", zap.String("note_path", event.RelPath), zap.Error(err))
		}

		for _, h := range noDirectiveHooks {
			_, err = r.executeHookSync(h, "post_update_note", evs, nil, event.RelPath, event.DirectoryID.String(), dirRelPath, "")
			if err != nil {
				r.logger.Error("failed to execute no-directive post_update_note hook", zap.String("hook_id", h.ID), zap.Error(err))
			}
		}
	}
}

func (r *Runner) executeNoteDirectives(ctx context.Context, dirID scalar.ID, dirRelPath string, relPath string, content string, triggerType string, filterHookID scalar.ID) (bool, error) {
	fastCheck := fastCheckReg.MatchString(content)
	contentSummary := content
	if len(contentSummary) > 500 {
		contentSummary = contentSummary[:500] + "...(truncated)"
	}
	r.logger.Debug("executeNoteDirectives start",
		zap.String("relPath", relPath),
		zap.String("triggerType", triggerType),
		zap.String("filterHookID", filterHookID.String()),
		zap.Bool("fastCheck", fastCheck),
		zap.String("contentHead", contentSummary),
	)
	// 快速粗筛：如果文本内没有任何可能匹配指令的行，直接退出，零磁盘 I/O 浪费
	if !fastCheck {
		return false, nil
	}

	hooks, err := r.LoadHooks()
	if err != nil {
		return false, err
	}

	hookMap := make(map[string]HookConfig)
	var registeredDirectives []string
	for _, h := range hooks {
		if h.Directive != nil && h.Directive.Name != "" {
			if existing, ok := hookMap[h.Directive.Name]; ok {
				r.logger.Error("duplicate directive name detected, the later hook will override the earlier one",
					zap.String("directive", h.Directive.Name),
					zap.String("existing_hook_id", existing.ID),
					zap.String("new_hook_id", h.ID),
				)
			}
			hookMap[h.Directive.Name] = h
			registeredDirectives = append(registeredDirectives, h.Directive.Name)
		}
	}
	r.logger.Debug("executeNoteDirectives hookMap built", zap.Strings("directives", registeredDirectives))

	// 1. 获取临时的 hook-run-id
	runID := getHookRunID(content)
	var isKnown bool
	if runID != "" {
		r.muTasks.Lock()
		_, isKnown = r.activeTasks[runID]
		r.muTasks.Unlock()
	}

	noteAbsPath := filepath.Join(r.rootDir, relPath)

	// 1b. 若是有已知且正在运行的 ID，则进行快速返回或执行后置迟到擦除
	if runID != "" && isKnown {
		isBefore3, failedDirectives := func() (bool, map[string]bool) {
			r.muTasks.Lock()
			defer r.muTasks.Unlock()
			task := r.activeTasks[runID]
			if task.phase == phaseBefore3 {
				task.paths[noteAbsPath] = struct{}{}
				return true, nil
			}
			fd := make(map[string]bool)
			if task.failedDirectives != nil {
				maps.Copy(fd, task.failedDirectives)
			}
			return false, fd
		}()

		if isBefore3 {
			r.logger.Debug("associated new path with active hook-run-id", zap.String("path", relPath), zap.String("run_id", runID))
			return false, nil
		}

		// 1b2. 若步骤 3 已完毕，执行后置迟到擦除
		r.logger.Debug("executing late path cleanup for known hook-run-id", zap.String("path", relPath), zap.String("run_id", runID))
		r.postProcessNoteDirectives(ctx, noteAbsPath, runID, triggerType, hookMap, failedDirectives)
		return false, nil
	}

	// 2. 对于新启动的指令流程，我们先匹配提取 pending 任务
	type pendingHook struct {
		config      HookConfig
		triggerType string
		events      []HookEvent
		args        []string
		relPath     string
		dirID       string
		dirRelPath  string
		action      string // 已解析的操作（COMMENT_OUT/REMOVE/KEEP），在钩子执行完成后设置
	}

	var pending []pendingHook

	// 通过正则遍历提取指令到 pending 列表中，内容不做任何抹除以供 Hook 脚本使用
	matches := directiveReg.FindAllStringSubmatch(content, -1)
	r.logger.Debug("executeNoteDirectives regexp matched lines", zap.Int("count", len(matches)))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		matchedLine := match[0]
		cmdName := match[1]
		cmdArgs := ""
		if len(match) > 2 {
			cmdArgs = strings.TrimSpace(match[2])
		}

		r.logger.Debug("executeNoteDirectives processing line",
			zap.String("matchedLine", strings.TrimSpace(matchedLine)),
			zap.String("cmdName", cmdName),
			zap.String("cmdArgs", cmdArgs),
		)

		hookConfig, ok := hookMap[cmdName]
		if !ok {
			r.logger.Debug("executeNoteDirectives cmdName not found in hookMap", zap.String("cmdName", cmdName))
			continue
		}

		// 根据触发类型进行筛选
		filterPassed := false
		switch triggerType {
		case "post_update_note":
			if hookConfig.On.PostUpdateNote == nil {
				r.logger.Debug("executeNoteDirectives trigger filter skipped: On.PostUpdateNote is nil")
				continue
			}
			// 若设定为 true，说明它是无条件静默触发，指令阶段不重复触发
			if hookConfig.On.PostUpdateNote.IgnoreDirective {
				r.logger.Debug("executeNoteDirectives trigger filter skipped: On.PostUpdateNote.IgnoreDirective is true")
				continue
			}
			filterPassed = true
		case "post_commit_session":
			if hookConfig.On.PostCommitSession == nil || hookConfig.On.PostCommitSession.NoteScan == nil {
				r.logger.Debug("executeNoteDirectives trigger filter skipped: On.PostCommitSession.NoteScan is nil")
				continue
			}
			// 若设定为 true，说明它是无条件静默扫描，指令阶段不重复触发
			if hookConfig.On.PostCommitSession.NoteScan.IgnoreDirective {
				r.logger.Debug("executeNoteDirectives trigger filter skipped: On.PostCommitSession.NoteScan.IgnoreDirective is true")
				continue
			}
			filterPassed = true
		case "note_dispatch":
			if hookConfig.On.NoteDispatch == nil {
				r.logger.Debug("executeNoteDirectives trigger filter skipped: On.NoteDispatch is nil")
				continue
			}
			filterPassed = true
		default:
			r.logger.Debug("executeNoteDirectives trigger filter skipped: unknown triggerType")
			continue
		}

		r.logger.Debug("executeNoteDirectives trigger filter passed", zap.Bool("passed", filterPassed))

		if !filterHookID.IsZero() {
			domH := domhook.FromRepository(hookConfig.ID, hookConfig.Name, hookConfig.Description, hookConfig.On.ImageDispatch != nil, hookConfig.On.NoteDispatch != nil, nil, false, false)
			idMatch := domH.ID() == filterHookID
			r.logger.Debug("executeNoteDirectives hook ID filter check",
				zap.String("domH.ID", domH.ID().String()),
				zap.String("filterHookID", filterHookID.String()),
				zap.Bool("matched", idMatch),
			)
			if !idMatch {
				continue
			}
		}

		// 寻找配套的图片
		evs, err := r.findAssociatedImageEvents(ctx, relPath)
		if err != nil {
			r.logger.Error("failed to get associated image for note directive", zap.String("note_path", relPath), zap.Error(err))
			continue
		}

		var args []string
		if cmdArgs != "" {
			args = splitArgs(cmdArgs)
		}

		r.logger.Debug("executeNoteDirectives appending pending directive",
			zap.String("hookID", hookConfig.ID),
			zap.Strings("args", args),
		)

		// 将此任务入队暂存，暂不执行
		pending = append(pending, pendingHook{
			config:      hookConfig,
			triggerType: triggerType,
			events:      evs,
			args:        args,
			relPath:     relPath,
			dirID:       dirID.String(),
			dirRelPath:  dirRelPath,
		})
	}

	r.logger.Debug("executeNoteDirectives loop finished", zap.Int("pendingCount", len(pending)))
	if len(pending) == 0 {
		return false, nil
	}

	// 3. 确定有 pending 指令要运行，我们才在 frontmatter 设置新随机 runID 并写入
	runID = fmt.Sprintf("run_%019d_%06d", time.Now().UnixNano(), rand.Intn(1000000))
	newContent := setHookRunID(content, runID)
	if newContent != content {
		newContentBytes := []byte(newContent)
		if err := r.writeFileWithIgnore(noteAbsPath, newContentBytes, 0644); err != nil {
			r.logger.Error("failed to write hook-run-id to note", zap.String("path", relPath), zap.Error(err))
			return false, err
		}
		content = newContent
	}

	// 在内存中注册防重入路径
	r.muTasks.Lock()
	task, exists := r.activeTasks[runID]
	if !exists {
		task = &activeTask{
			phase: phaseBefore3,
			paths: make(map[string]struct{}),
		}
		r.activeTasks[runID] = task
	}
	task.paths[noteAbsPath] = struct{}{}
	r.muTasks.Unlock()

	defer func() {
		time.AfterFunc(10*time.Second, func() {
			r.muTasks.Lock()
			delete(r.activeTasks, runID)
			r.muTasks.Unlock()
		})
	}()

	// 4. 执行斜杠指令 (注入唯一的 hook-run-id)
	for i, p := range pending {
		action, err := r.executeHookSync(p.config, p.triggerType, p.events, p.args, p.relPath, p.dirID, p.dirRelPath, runID)
		if err != nil {
			r.logger.Error("failed to execute hook for directive", zap.String("hook_id", p.config.ID), zap.Error(err))
		}
		pending[i].action = action
	}

	// 5. 执行完成后，在 activeTask 关联的所有路径上执行擦除
	r.muTasks.Lock()
	r.activeTasks[runID].phase = phaseAfter3
	failedDirectives := make(map[string]bool)
	for _, p := range pending {
		failedDirectives[p.config.Directive.Name] = p.action == p.config.Directive.OnFailAction
	}
	r.activeTasks[runID].failedDirectives = failedDirectives
	var pathsToProcess []string
	for p := range r.activeTasks[runID].paths {
		pathsToProcess = append(pathsToProcess, p)
	}
	r.muTasks.Unlock()

	for _, p := range pathsToProcess {
		contentBytes, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			r.logger.Error("failed to read file during directive cleanup", zap.String("path", p), zap.Error(err))
			continue
		}

		fileContent := string(contentBytes)
		currentID := getHookRunID(fileContent)
		if currentID != runID {
			continue // 3b. 如果不一致，说明该路径已被覆盖，忽略之
		}

		// 3a. 擦除指令和 hook-run-id 并写入文件
		idx := 0
		finalContent := directiveReg.ReplaceAllStringFunc(fileContent, func(matchedLine string) string {
			if idx >= len(pending) {
				return matchedLine
			}
			pTask := pending[idx]
			idx++

			return applyDirectiveAction(pTask.action, matchedLine)
		})

		// 擦除 hook-run-id
		finalContent = removeHookRunID(finalContent)

		if finalContent != fileContent {
			if finalContent == "" {
				// 处理后的内容为空，直接删除文件
				if err := os.Remove(p); err != nil {
					r.logger.Error("failed to delete empty note file after directive cleanup", zap.String("path", p), zap.Error(err))
				}
				continue
			}
			// 在写回磁盘前，计算 xxhash 并注册为忽略事件，自防循环
			finalContentBytes := []byte(finalContent)
			if err := r.writeFileWithIgnore(p, finalContentBytes, 0644); err != nil {
				r.logger.Error("failed to write clean content to note file during cleanup", zap.String("path", p), zap.Error(err))
			}
		}
	}

	return true, nil
}

// executeHookSync 同步执行钩子并返回解析后的操作
// 返回的操作已解析：成功时优先使用脚本覆盖值，否则使用 on_success_action；失败时使用 on_fail_action
func (r *Runner) executeHookSync(hook HookConfig, triggerName string, events []HookEvent, extraArgs []string, notePath string, dirID string, dirRel string, runID string) (string, error) {
	if runID == "" {
		runID = fmt.Sprintf("run_%019d_%06d", time.Now().UnixNano(), rand.Intn(1000000))
	}
	resChan := make(chan HookExecutionResult, 1)

	// 构建完整命令行，对含空格/引号的参数用引号包裹
	var cmdParts []string
	cmdParts = append(cmdParts, hook.Command)
	for _, arg := range extraArgs {
		if strings.Contains(arg, " ") || strings.Contains(arg, "\"") {
			escaped := strings.ReplaceAll(arg, "\"", "\\\"")
			cmdParts = append(cmdParts, fmt.Sprintf("\"%s\"", escaped))
		} else {
			cmdParts = append(cmdParts, arg)
		}
	}
	fullCommand := strings.Join(cmdParts, " ")

	r.ch <- HookExecutionTask{
		HookID:       hook.ID,
		HookName:     hook.Name,
		Command:      fullCommand,
		ExtraArgs:    extraArgs,
		TriggerName:  triggerName,
		Events:       events,
		Dir:          hook.Dir,
		Env:          hook.Env,
		NotePath:     notePath,
		DirectoryID:  dirID,
		DirectoryRel: dirRel,
		ResultChan:   resChan,
		RunID:        runID,
	}

	select {
	case result := <-resChan:
		if result.Error != nil {
			// 失败时总是使用 on_fail_action
			if hook.Directive != nil {
				return hook.Directive.OnFailAction, result.Error
			}
			return "", result.Error
		}
		// 成功：脚本覆盖优先，否则使用 on_success_action
		if result.Action != "" {
			return result.Action, nil
		}
		if hook.Directive != nil {
			return hook.Directive.OnSuccessAction, nil
		}
		return "", nil
	case <-r.ctx.Done():
		return "", r.ctx.Err()
	}
}

func (r *Runner) handleMetadataUpdated(event *shared.MetadataUpdatedEvent) {
	hooks, err := r.LoadHooks()
	if err != nil {
		r.logger.Error("failed to load hooks during metadata updated event", zap.Error(err))
		return
	}

	for _, h := range hooks {
		trigger := h.On.PostUpdateImageMetadata
		if trigger == nil {
			continue
		}

		// 直接评估事件是否匹配钩子的过滤规则
		if !trigger.Match(event) {
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

// Filters 元数据更新筛选条件，专门用于外部钩子中的事件过滤
type Filters struct {
	ID     []scalar.ID `toml:"id"`
	Rating []int       `toml:"rating"`
	Label  []string    `toml:"label"`
	Query  string      `toml:"query"`
}

// Match 评估单个事件是否匹配当前的筛选条件
func (f *Filters) Match(event *shared.MetadataUpdatedEvent) bool {
	if f == nil {
		return true
	}

	if len(f.ID) > 0 && !slices.Contains(f.ID, event.ID) {
		return false
	}

	if len(f.Rating) > 0 && !slices.Contains(f.Rating, event.Rating) {
		return false
	}

	if len(f.Label) > 0 {
		matched := false
		eventLabelLower := strings.ToLower(event.Label)
		for _, l := range f.Label {
			if strings.ToLower(l) == eventLabelLower {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	if f.Query != "" {
		queryLower := strings.ToLower(f.Query)
		filename := filepath.Base(event.Path)
		if !strings.Contains(strings.ToLower(filename), queryLower) {
			return false
		}
	}

	return true
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
		dec := toml.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&cfg); err != nil {
			r.logger.Warn("failed to parse hook config toml", zap.String("path", path), zap.Error(err))
			continue
		}

		cfg.Dir = filepath.Dir(path) // 记录 Hook 配置文件所在目录

		if cfg.Directive != nil {
			if cfg.Directive.OnSuccessAction == "" {
				cfg.Directive.OnSuccessAction = "COMMENT_OUT"
			}
			if cfg.Directive.OnFailAction == "" {
				cfg.Directive.OnFailAction = "COMMENT_OUT"
			}
		}

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

	cmd := newHookCmd(ctx, task.Command)

	cmd.Dir = task.Dir // 将脚本的工作目录设置为 Hook 配置文件所在的目录

	// 生成临时文件路径供脚本通过 IMAGE_FUNNEL_ACTION 写入覆盖操作，不提前创建文件
	actionFilePath := filepath.Join(os.TempDir(), fmt.Sprintf("image_funnel_action_%s.txt", task.RunID))

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
		"IMAGE_FUNNEL_ROOT_DIR="+r.rootDir,
		"IMAGE_FUNNEL_ACTION="+actionFilePath,
		"PYTHONIOENCODING=utf-8",
		"PYTHONUTF8=1",
	)

	if task.NotePath != "" {
		noteAbsPath := filepath.Join(r.rootDir, task.NotePath)
		mPathsJSON, _ := json.Marshal([]string{noteAbsPath})
		env = append(env, "IMAGE_FUNNEL_NOTE_PATHS="+string(mPathsJSON))
	}
	if task.DirectoryID != "" {
		env = append(env, "IMAGE_FUNNEL_DIRECTORY_ID="+task.DirectoryID)
	}
	if task.DirectoryRel != "" {
		env = append(env, "IMAGE_FUNNEL_DIRECTORY_REL_PATH="+task.DirectoryRel)
	}
	env = append(env, "IMAGE_FUNNEL_HOOK_RUN_ID="+task.RunID)

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

	// 清理临时文件
	defer func() {
		if removeErr := os.Remove(actionFilePath); removeErr != nil && !os.IsNotExist(removeErr) {
			r.logger.Warn("failed to clean up action file", zap.String("path", actionFilePath), zap.Error(removeErr))
		}
	}()

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
				task.ResultChan <- HookExecutionResult{Error: fmt.Errorf("hook script failed: %w, stderr: %s", err, stderrStr)}
			} else {
				task.ResultChan <- HookExecutionResult{Error: fmt.Errorf("hook script failed: %w", err)}
			}
		}
		return
	}

	r.logger.Info("external hook command completed",
		zap.String("hook_id", task.HookID),
		zap.Duration("duration", duration),
		zap.String("stdout", stdout.String()),
	)
	// 开发环境通过 Debug 级别输出 stderr，生产环境自动过滤
	if stderrStr := stderr.String(); stderrStr != "" {
		r.logger.Debug("external hook stderr",
			zap.String("hook_id", task.HookID),
			zap.String("stderr", stderrStr),
		)
	}

	// 读取脚本通过 IMAGE_FUNNEL_ACTION 写入的覆盖操作
	var overrideAction string
	data, readErr := os.ReadFile(actionFilePath)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			// 文件不存在 = 脚本未覆盖，正常路径
		} else {
			errMsg := fmt.Sprintf("failed to read IMAGE_FUNNEL_ACTION file: %v", readErr)
			r.logger.Error(errMsg, zap.String("hook_id", task.HookID), zap.String("path", actionFilePath))
			if task.ResultChan != nil {
				task.ResultChan <- HookExecutionResult{Error: fmt.Errorf("%s", errMsg)}
			}
			return
		}
	} else {
		overrideAction = strings.TrimSpace(string(data))
		if overrideAction != "" && !isValidDirectiveAction(overrideAction) {
			errMsg := fmt.Sprintf("unsupported action in IMAGE_FUNNEL_ACTION file: %q", overrideAction)
			r.logger.Error(errMsg, zap.String("hook_id", task.HookID))
			if task.ResultChan != nil {
				task.ResultChan <- HookExecutionResult{Error: fmt.Errorf("%s", errMsg)}
			}
			return
		}
	}

	if task.ResultChan != nil {
		task.ResultChan <- HookExecutionResult{Action: overrideAction}
	}
}

// isValidDirectiveAction 检查操作是否为支持的指令操作
func isValidDirectiveAction(action string) bool {
	switch action {
	case "COMMENT_OUT", "REMOVE", "KEEP":
		return true
	default:
		return false
	}
}

func toDomainDirective(cfg *DirectiveConfig) *domhook.Directive {
	if cfg == nil {
		return nil
	}
	return &domhook.Directive{
		Name:            cfg.Name,
		Usage:           cfg.Usage,
		OnSuccessAction: cfg.OnSuccessAction,
		OnFailAction:    cfg.OnFailAction,
	}
}

func (r *Runner) OnCommitSession(ctx context.Context, dirID scalar.ID, dirRelPath string) error {
	// 触发异步后台任务，尽快返回给调用者
	go func() {
		hooks, err := r.LoadHooks()
		if err != nil {
			r.logger.Error("failed to load hooks for post_commit_session in background", zap.Error(err))
			return
		}

		// 1. 触发纯会话提交钩子 (配置了 post_commit_session 但没有配置 note_scan 属性的钩子)
		var pureCommitHooks []HookConfig
		for _, h := range hooks {
			if h.On.PostCommitSession != nil && h.On.PostCommitSession.NoteScan == nil {
				pureCommitHooks = append(pureCommitHooks, h)
			}
		}

		if len(pureCommitHooks) > 0 {
			// 彻底禁止为了钩子加载目录下所有图片！仅传入空列表和会话目录信息，由脚本端自行 GraphQL 按需过滤
			for _, h := range pureCommitHooks {
				_, err = r.executeHookSync(h, "post_commit_session", nil, nil, "", dirID.String(), dirRelPath, "")
				if err != nil {
					r.logger.Error("failed to execute pure post_commit_session hook", zap.String("hook_id", h.ID), zap.Error(err))
				}
			}
		}

		// 2. 扫描笔记文件并处理
		dirAbsPath := filepath.Join(r.rootDir, dirRelPath)
		entries, err := os.ReadDir(dirAbsPath)
		if err != nil {
			r.logger.Error("failed to read directory for post_commit_session note scan", zap.String("dir_rel_path", dirRelPath), zap.Error(err))
			return
		}

		for _, entry := range entries {
			if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".md" {
				continue
			}

			noteRelPath := filepath.ToSlash(filepath.Join(dirRelPath, entry.Name()))
			noteAbsPath := filepath.Join(r.rootDir, noteRelPath)

			contentBytes, err := os.ReadFile(noteAbsPath)
			if err != nil {
				r.logger.Error("failed to read note file during commit scan", zap.String("path", noteRelPath), zap.Error(err))
				continue
			}

			// 2a. 带有指令的钩子：解析与执行指令
			_, err = r.executeNoteDirectives(r.ctx, dirID, dirRelPath, noteRelPath, string(contentBytes), "post_commit_session", scalar.ID{})
			if err != nil {
				r.logger.Error("failed to process note directives during commit scan", zap.String("path", noteRelPath), zap.Error(err))
				continue
			}

			// processSingleNote 内部已完成相应的预先写回和失败回滚

			// 2b. 无指令的 note_scan 钩子：配置了 note_scan 且没有 Directive 或 ignore_directive = true 时直接触发
			var noDirectiveNoteScanHooks []HookConfig
			for _, h := range hooks {
				if h.On.PostCommitSession != nil && h.On.PostCommitSession.NoteScan != nil {
					if h.Directive == nil || h.On.PostCommitSession.NoteScan.IgnoreDirective {
						noDirectiveNoteScanHooks = append(noDirectiveNoteScanHooks, h)
					}
				}
			}

			if len(noDirectiveNoteScanHooks) > 0 {
				evs, err := r.findAssociatedImageEvents(r.ctx, noteRelPath)
				if err != nil {
					r.logger.Error("failed to get associated image for commit scan hook", zap.String("note_path", noteRelPath), zap.Error(err))
				}
				for _, h := range noDirectiveNoteScanHooks {
					_, err = r.executeHookSync(h, "post_commit_session", evs, nil, noteRelPath, dirID.String(), dirRelPath, "")
					if err != nil {
						r.logger.Error("failed to execute no-directive post_commit_session note_scan hook", zap.String("hook_id", h.ID), zap.Error(err))
					}
				}
			}
		}
	}()

	// 立即返回，不等待后台任务完成
	return nil
}

// applyDirectiveAction 根据指令动作（REMOVE/KEEP/COMMENT_OUT）返回替换后的文本行
func applyDirectiveAction(action string, matchedLine string) string {
	switch action {
	case "REMOVE":
		return ""
	case "KEEP":
		return matchedLine
	default: // COMMENT_OUT
		var newline string
		if strings.HasSuffix(matchedLine, "\r\n") {
			newline = "\r\n"
		} else if strings.HasSuffix(matchedLine, "\n") {
			newline = "\n"
		}
		lineWithoutNL := strings.TrimSuffix(matchedLine, newline)
		trimmed := strings.TrimSpace(lineWithoutNL)
		return fmt.Sprintf("%%%% %s %%%%"+newline, trimmed)
	}
}

// splitArgs 按空白分割参数字符串，支持双引号包裹含空格的参数
func splitArgs(s string) []string {
	var args []string
	var current strings.Builder
	inQuotes := false

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '"' {
			inQuotes = !inQuotes
		} else if (ch == ' ' || ch == '\t') && !inQuotes {
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		} else {
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		args = append(args, current.String())
	}
	return args
}

func associatedImageRelPath(noteRelPath string) (string, bool) {
	ext := filepath.Ext(noteRelPath)
	if ext == "" {
		return "", false
	}
	return strings.TrimSuffix(noteRelPath, ext), true
}

// findAssociatedImageEvents 查找笔记配套的图片，构建对应的 HookEvent 列表
func (r *Runner) findAssociatedImageEvents(ctx context.Context, noteRelPath string) ([]HookEvent, error) {
	imgRelPath, ok := associatedImageRelPath(noteRelPath)
	if !ok {
		return nil, nil
	}
	img, err := r.imgRepo.Get(ctx, imgRelPath)
	if err != nil {
		img, err = apperror.IgnoreNotFound(img, err)
		if err != nil {
			return nil, err
		}
	}
	if img == nil {
		return nil, nil
	}
	return []HookEvent{{
		ID:   img.ID().String(),
		Path: filepath.Join(r.rootDir, img.RelPath()),
	}}, nil
}

// parseFrontmatter 提取文件的 frontmatter 和 body。
// 如果没有 frontmatter，返回 "", content, false, newline
func parseFrontmatter(content string) (frontmatter, body string, has bool, newline string) {
	newline = "\n"
	if strings.Contains(content, "\r\n") {
		newline = "\r\n"
	}

	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return "", content, false, newline
	}

	parts := strings.SplitN(normalized, "---\n", 3)
	if len(parts) < 3 {
		return "", content, false, newline
	}

	// 统一用文件本身的换行符拼接
	frontmatter = strings.ReplaceAll(parts[1], "\n", newline)
	body = strings.ReplaceAll(parts[2], "\n", newline)
	return frontmatter, body, true, newline
}

func getHookRunID(content string) string {
	fm, _, has, newline := parseFrontmatter(content)
	if !has {
		return ""
	}
	lines := strings.SplitSeq(fm, newline)
	for line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		kv := strings.SplitN(trimmed, ":", 2)
		if len(kv) == 2 {
			k := strings.ToLower(strings.TrimSpace(kv[0]))
			if k == "hook-run-id" {
				return strings.TrimSpace(kv[1])
			}
		}
	}
	return ""
}

func setHookRunID(content string, runID string) string {
	fm, body, has, newline := parseFrontmatter(content)
	if !has {
		return fmt.Sprintf("---%[1]shook-run-id: %[2]s%[1]s---%[1]s%[3]s", newline, runID, content)
	}

	var sb strings.Builder
	var found bool
	for line := range strings.SplitSeq(fm, newline) {
		trimmed := strings.TrimSpace(line)
		if !found && trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			kv := strings.SplitN(trimmed, ":", 2)
			if len(kv) == 2 {
				k := strings.ToLower(strings.TrimSpace(kv[0]))
				if k == "hook-run-id" {
					sb.WriteString("hook-run-id: ")
					sb.WriteString(runID)
					sb.WriteString(newline)
					found = true
					continue
				}
			}
		}
		sb.WriteString(line)
		sb.WriteString(newline)
	}

	if !found {
		// 如果未找到，我们直接在最前头插入
		return fmt.Sprintf("---%[1]shook-run-id: %[2]s%[1]s%[3]s---%[1]s%[4]s", newline, runID, sb.String(), body)
	}

	fmStr := strings.TrimSuffix(sb.String(), newline)
	return fmt.Sprintf("---%[1]s%[2]s%[1]s---%[1]s%[3]s", newline, fmStr, body)
}

func removeHookRunID(content string) string {
	fm, body, has, newline := parseFrontmatter(content)
	if !has {
		return content
	}

	var sb strings.Builder
	var hasActualLines bool
	for line := range strings.SplitSeq(fm, newline) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		kv := strings.SplitN(trimmed, ":", 2)
		if len(kv) == 2 {
			k := strings.ToLower(strings.TrimSpace(kv[0]))
			if k == "hook-run-id" {
				continue // 过滤移除 hook-run-id 这一行
			}
		}
		hasActualLines = true
		sb.WriteString(line)
		sb.WriteString(newline)
	}

	if !hasActualLines {
		// 剥离整个 frontmatter
		return body
	}

	fmStr := strings.TrimSuffix(sb.String(), newline)
	return fmt.Sprintf("---%[1]s%[2]s%[1]s---%[1]s%[3]s", newline, fmStr, body)
}

func (r *Runner) hashContent(content []byte) uint64 {
	return xxhash.Sum64(content)
}

func (r *Runner) addWriteIgnore(absPath string, contentHash uint64, duration time.Duration) {
	r.muIgnore.Lock()
	defer r.muIgnore.Unlock()

	r.writeIgnore[absPath] = writeIgnoreItem{
		contentHash: contentHash,
		expireTime:  time.Now().Add(duration),
	}

	// 轻量清理过期的忽略哈希项
	now := time.Now()
	for path, item := range r.writeIgnore {
		if now.After(item.expireTime) {
			delete(r.writeIgnore, path)
		}
	}
}

func (r *Runner) shouldIgnoreEvent(absPath string, content []byte) bool {
	r.muIgnore.Lock()
	defer r.muIgnore.Unlock()

	item, exists := r.writeIgnore[absPath]
	if !exists {
		return false
	}
	if time.Now().After(item.expireTime) {
		delete(r.writeIgnore, absPath)
		return false
	}

	return item.contentHash == r.hashContent(content)
}

// writeFileWithIgnore 写入文件前注册防重入哈希，避免自身写入触发文件变更事件
func (r *Runner) writeFileWithIgnore(absPath string, content []byte, perm os.FileMode) error {
	r.addWriteIgnore(absPath, r.hashContent(content), 10*time.Second)
	return os.WriteFile(absPath, content, perm)
}

func (r *Runner) postProcessNoteDirectives(ctx context.Context, absPath string, runID string, triggerType string, hookMap map[string]HookConfig, failedDirectives map[string]bool) {
	contentBytes, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		r.logger.Error("failed to read file during late task cleanup", zap.String("path", absPath), zap.Error(err))
		return
	}

	fileContent := string(contentBytes)
	currentID := getHookRunID(fileContent)
	if currentID != runID {
		return
	}

	finalContent := directiveReg.ReplaceAllStringFunc(fileContent, func(matchedLine string) string {
		matches := directiveReg.FindStringSubmatch(matchedLine)
		if len(matches) < 2 {
			return matchedLine
		}
		cmdName := matches[1]
		hookConfig, ok := hookMap[cmdName]
		if !ok {
			return matchedLine
		}

		action := hookConfig.Directive.OnSuccessAction
		if failedDirectives != nil && failedDirectives[cmdName] {
			action = hookConfig.Directive.OnFailAction
		}
		return applyDirectiveAction(action, matchedLine)
	})

	finalContent = removeHookRunID(finalContent)

	if finalContent != fileContent {
		if finalContent == "" {
			// 处理后的内容为空，直接删除文件
			if err := os.Remove(absPath); err != nil {
				r.logger.Error("failed to delete empty note file during late cleanup", zap.String("path", absPath), zap.Error(err))
			}
			return
		}
		finalContentBytes := []byte(finalContent)
		if err := r.writeFileWithIgnore(absPath, finalContentBytes, 0644); err != nil {
			r.logger.Error("failed to write clean content to note file during late cleanup", zap.String("path", absPath), zap.Error(err))
		}
	}
}
