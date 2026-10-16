package hook

import (
	"context"
	"sync"
	"time"

	"main/internal/domain/device"
	"main/internal/domain/directory"
	domhook "main/internal/domain/hook"
	"main/internal/domain/image"
	"main/internal/domain/relpath"
	"main/internal/pubsub"
	"main/internal/scalar"
	"main/internal/shared"

	"go.uber.org/zap"
)

// DebounceDurationDefault 元数据更新路径的默认防抖合批时长：
// 批量 XMP 写入会产生密集事件，合批避免同一批写入触发多次脚本冷启动
const DebounceDurationDefault = 100 * time.Millisecond

// Deps Runner 所需的全部依赖。字段全部必填，缺失即构造失败——
// 依赖缺失意味着某条触发路径会在运行期才暴露为静默降级，构造期失败更容易定位。
type Deps struct {
	RootDir  string
	HooksDir string
	DataDir  string
	// GraphQLURL 本机 GraphQL 监听地址，脚本经该地址回调本应用
	GraphQLURL         string
	Logger             *zap.Logger
	MetadataUpdatedSub pubsub.Topic[*shared.MetadataUpdatedEvent]
	FileChangedSub     pubsub.Topic[*directory.FileChangedEvent]
	TokenSource        device.TokenSource
	ImageRepo          image.Repository
	DirRepo            directory.Repository
	NotificationSender shared.NotificationSender
}

// seams 触发路径的可替换实现。生产环境由 defaultSeams 构建默认实现，
// 测试从这些 seam 注入替身，而不是从 Runner 构造器的位置参数注入。
type seams struct {
	// Config 配置来源：逐次读盘解析，保留「改 toml 即时生效」语义
	Config configSource
	// Process 进程原语：构造进程 + 运行 + 取消，覆盖单次执行与常驻 JSON-RPC 两条生命周期
	Process processRunner
	// DebounceDuration 元数据更新路径的防抖合批时长
	DebounceDuration time.Duration
	// AutocompleteTimeout 单次补全请求的超时兜底时长
	AutocompleteTimeout time.Duration
	// AutocompleteIdleTimeout 常驻补全进程的空闲回收时长
	AutocompleteIdleTimeout time.Duration
}

// defaultSeams 基于依赖构建生产环境的默认实现
func defaultSeams(deps Deps) seams {
	return seams{
		Config:                  newTOMLConfigSource(deps.HooksDir, deps.Logger),
		Process:                 execProcessRunner{log: deps.Logger},
		DebounceDuration:        DebounceDurationDefault,
		AutocompleteTimeout:     autocompleteDefaultTimeout,
		AutocompleteIdleTimeout: autocompleteDefaultIdleTimeout,
	}
}

var _ domhook.Repository = (*Runner)(nil)
var _ domhook.Runner = (*Runner)(nil)

// Runner 外部钩子管理器：唯一订阅事件总线的编排者，收到事件后路由给对应触发路径。
// 每条触发路径只接受调用、产出执行结果，彼此不感知对方的存在。
type Runner struct {
	deps   Deps
	config configSource

	bgCtx   context.Context
	cancel  context.CancelFunc
	exec    *hookExecutor
	rewrite *noteRewriter

	metadataTrigger *imageMetadataTrigger
	noteUpdate      *noteUpdateTrigger
	sessionCommit   *sessionCommitTrigger
	imageDispatch   *imageDispatchTrigger
	noteDispatch    *noteDispatchTrigger
	autocomplete    *autocompleteTrigger
	copyContent     *copyTrigger

	wg        sync.WaitGroup
	closeOnce sync.Once
}

// NewRunner 构建外部钩子管理器并启动事件监听与后台 worker。
// 全部依赖必填；进程原语与配置来源使用生产实现，由测试经 newRunner 注入替身。
func NewRunner(deps Deps) *Runner {
	return newRunner(deps, defaultSeams(deps))
}

// newRunner 以指定的 seam 实现构建 Runner，供测试注入替身
func newRunner(deps Deps, sm seams) *Runner {
	validateDeps(deps)

	bgCtx, cancel := context.WithCancel(context.Background())
	notes := newNoteContext(deps.RootDir, deps.ImageRepo, deps.DirRepo)
	env := &envBuilder{
		rootDir:     deps.RootDir,
		dataDir:     deps.DataDir,
		graphqlURL:  deps.GraphQLURL,
		tokenSource: deps.TokenSource,
		dirRepo:     deps.DirRepo,
	}
	notifier := newHookNotifier(deps.NotificationSender, deps.Logger)
	guard := newWriteGuard()

	exec := newHookExecutor(bgCtx, sm.Process, env, notifier, deps.Logger, deps.RootDir, deps.HooksDir)
	// 指令改写器在「笔记变更」「会话提交」「手动笔记派发」三条路径间共享运行状态，
	// 必须是同一实例，否则迟到的文件事件找不到对应的运行记录
	rewrite := newNoteRewriter(sm.Config, exec, notes, guard, notifier, deps.RootDir, deps.Logger)

	r := &Runner{
		deps:            deps,
		config:          sm.Config,
		bgCtx:           bgCtx,
		cancel:          cancel,
		exec:            exec,
		rewrite:         rewrite,
		metadataTrigger: newImageMetadataTrigger(bgCtx, sm.Config, exec, notes, sm.DebounceDuration, deps.Logger),
		noteUpdate:      newNoteUpdateTrigger(bgCtx, deps.RootDir, sm.Config, exec, rewrite, guard, notes, deps.Logger),
		sessionCommit:   newSessionCommitTrigger(bgCtx, deps.RootDir, sm.Config, exec, rewrite, notes, deps.Logger),
		imageDispatch:   newImageDispatchTrigger(sm.Config, exec, notes),
		noteDispatch:    newNoteDispatchTrigger(deps.RootDir, sm.Config, exec, rewrite, notes),
		autocomplete:    newAutocompleteTrigger(bgCtx, deps.RootDir, deps.HooksDir, sm.Config, env, notes, sm.Process, sm.AutocompleteIdleTimeout, sm.AutocompleteTimeout, deps.Logger),
		copyContent:     newCopyTrigger(deps.RootDir, deps.HooksDir, sm.Config, env, sm.Process, deps.Logger),
	}

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.listen(bgCtx)
	}()

	return r
}

// validateDeps 校验全部依赖存在，缺失即 panic：
// Runner 是应用启动期构建的单例，构造失败比运行期静默降级更容易定位
func validateDeps(deps Deps) {
	required := []struct {
		name string
		ok   bool
	}{
		{"RootDir", deps.RootDir != ""},
		{"HooksDir", deps.HooksDir != ""},
		{"DataDir", deps.DataDir != ""},
		{"GraphQLURL", deps.GraphQLURL != ""},
		{"Logger", deps.Logger != nil},
		{"MetadataUpdatedSub", deps.MetadataUpdatedSub != nil},
		{"FileChangedSub", deps.FileChangedSub != nil},
		{"TokenSource", deps.TokenSource != nil},
		{"ImageRepo", deps.ImageRepo != nil},
		{"DirRepo", deps.DirRepo != nil},
		{"NotificationSender", deps.NotificationSender != nil},
	}
	for _, dep := range required {
		if !dep.ok {
			panic("hook.Deps." + dep.name + " is required")
		}
	}
}

// Close 结束监听与后台 worker，终止常驻进程与正在运行的钩子脚本
func (r *Runner) Close() {
	r.closeOnce.Do(func() {
		r.cancel()
		r.autocomplete.Close()
		r.exec.Close()
		r.wg.Wait()
	})
}

// listen 监听事件总线：文件变更交给笔记触发路径，元数据更新交给图片元数据触发路径
func (r *Runner) listen(ctx context.Context) {
	var inner sync.WaitGroup
	inner.Add(1)
	go func() {
		defer inner.Done()
		for event, err := range r.deps.FileChangedSub.Subscribe(ctx) {
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				continue
			}
			if err := r.noteUpdate.HandleFileChanged(event); err != nil {
				r.deps.Logger.Error("failed to handle file changed event", zap.Error(err))
			}
		}
	}()

	for event, err := range r.deps.MetadataUpdatedSub.Subscribe(ctx) {
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			continue
		}
		r.metadataTrigger.Handle(event)
	}

	// 元数据订阅已结束（仅在 ctx 取消时）等待文件变更订阅一并退出
	inner.Wait()
}

// #region 领域接口实现

func (r *Runner) List(ctx context.Context) ([]*domhook.Hook, error) {
	hooks, err := r.config.Load()
	if err != nil {
		return nil, err
	}
	var res []*domhook.Hook
	for _, h := range hooks {
		hasPostUpdateNote := h.On.PostUpdateNote != nil
		hasPostCommitSessionNoteScan := h.On.PostCommitSession != nil && h.On.PostCommitSession.NoteScan != nil
		var imageDispatchHotkeys []string
		if h.On.ImageDispatch != nil {
			imageDispatchHotkeys = h.On.ImageDispatch.Hotkeys
		}
		res = append(res, domhook.FromRepository(
			h.ID,
			h.Name,
			h.Description,
			h.On.ImageDispatch != nil,
			imageDispatchHotkeys,
			h.On.NoteDispatch != nil,
			toDomainDirective(h.Directive),
			hasPostUpdateNote,
			hasPostCommitSessionNoteScan,
		))
	}
	return res, nil
}

func (r *Runner) Trigger(ctx context.Context, ids []string, paths []string, hookID scalar.ID, triggerName string) error {
	return r.imageDispatch.Dispatch(ctx, ids, paths, hookID, triggerName)
}

// OnCommitSession 会话提交后触发钩子与笔记扫描。
// 保持 fire-and-forget 语义：立即返回，后台任务的生命周期随 Runner 结束而取消。
func (r *Runner) OnCommitSession(_ context.Context, dirRelPath relpath.Path) error {
	r.sessionCommit.Commit(dirRelPath)
	return nil
}

func (r *Runner) TriggerForNote(ctx context.Context, noteRelPath relpath.Path, hookID scalar.ID) error {
	return r.noteDispatch.Dispatch(ctx, noteRelPath, hookID)
}

func (r *Runner) Autocomplete(ctx context.Context, hookID scalar.ID, noteRelPath string, linePrefix string, query string) ([]*domhook.AutocompleteSuggestion, error) {
	return r.autocomplete.Complete(ctx, hookID, noteRelPath, linePrefix, query)
}

func (r *Runner) CopyContent(ctx context.Context, imageID scalar.ID, imageRelPath relpath.Path) (*domhook.CopyContent, error) {
	return r.copyContent.Content(ctx, imageID, imageRelPath)
}

// #endregion
