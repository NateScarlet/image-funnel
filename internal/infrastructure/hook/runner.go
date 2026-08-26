package hook

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"main/internal/domain/device"
	"main/internal/domain/directory"
	"main/internal/domain/hook"
	"main/internal/domain/image"
	"main/internal/pubsub"
	"main/internal/scalar"
	"main/internal/shared"

	"go.uber.org/zap"
)

// directoryService 本地目录服务接口
type directoryService interface {
	GetDirectory(ctx context.Context, id scalar.ID) (*directory.Directory, error)
}

var _ hook.Repository = (*Runner)(nil)
var _ hook.Runner = (*Runner)(nil)

// Runner 外部钩子管理器（应用/基础设施适配器服务）
type Runner struct {
	rootDir                 string
	hooksDir                string
	dataDir                 string
	logger                  *zap.Logger
	metadataUpdatedSub      pubsub.Topic[*shared.MetadataUpdatedEvent]
	fileChangedSub          pubsub.Topic[*shared.FileChangedEvent]
	graphqlURL              string
	tokenSource             device.TokenSource
	imgRepo                 image.Repository
	dirSvc                  directoryService
	dirRepo                 directory.Repository
	notifSender             shared.NotificationSender
	ch                      chan hookExecutionTask
	debouncer               *debouncer
	ctx                     context.Context
	cancel                  context.CancelFunc
	wg                      sync.WaitGroup
	muIgnore                sync.Mutex
	writeIgnore             map[string]writeIgnoreItem
	muTasks                 sync.Mutex
	activeTasks             map[string]*activeTask
	jsonrpcPool             *jsonrpcPool
	autocompleteTimeout     time.Duration
	autocompleteIdleTimeout time.Duration
	closeOnce               sync.Once
}

func NewRunner(
	rootDir string,
	hooksDir string,
	dataDir string,
	logger *zap.Logger,
	metadataUpdatedSub pubsub.Topic[*shared.MetadataUpdatedEvent],
	fileChangedSub pubsub.Topic[*shared.FileChangedEvent],
	graphqlURL string,
	tokenSource device.TokenSource,
	imgRepo image.Repository,
	dirSvc directoryService,
	dirRepo directory.Repository,
	notifSender shared.NotificationSender,
) *Runner {
	if notifSender == nil {
		panic("notifSender is required")
	}

	ctx, cancel := context.WithCancel(context.Background())
	r := &Runner{
		rootDir:                 rootDir,
		hooksDir:                hooksDir,
		dataDir:                 dataDir,
		logger:                  logger,
		metadataUpdatedSub:      metadataUpdatedSub,
		fileChangedSub:          fileChangedSub,
		graphqlURL:              graphqlURL,
		tokenSource:             tokenSource,
		imgRepo:                 imgRepo,
		dirSvc:                  dirSvc,
		dirRepo:                 dirRepo,
		notifSender:             notifSender,
		ch:                      make(chan hookExecutionTask, 1024),
		ctx:                     ctx,
		cancel:                  cancel,
		writeIgnore:             make(map[string]writeIgnoreItem),
		activeTasks:             make(map[string]*activeTask),
		autocompleteTimeout:     autocompleteDefaultTimeout,
		autocompleteIdleTimeout: autocompleteDefaultIdleTimeout,
	}

	r.debouncer = newDebouncer(100*time.Millisecond, r.onDebounceTrigger)

	r.jsonrpcPool = newJSONRPCPool(r.logger, r.autocompleteIdleTimeout, r.spawnJSONRPCProcess)

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
	r.closeOnce.Do(func() {
		r.cancel()
		r.jsonrpcPool.Close()
		close(r.ch)
		r.wg.Wait()
	})
}

func (r *Runner) List(ctx context.Context) ([]*hook.Hook, error) {
	hooks, err := r.loadHooks()
	if err != nil {
		return nil, err
	}
	var res []*hook.Hook
	for _, h := range hooks {
		hasPostUpdateNote := h.On.PostUpdateNote != nil
		hasPostCommitSessionNoteScan := h.On.PostCommitSession != nil && h.On.PostCommitSession.NoteScan != nil
		res = append(res, hook.FromRepository(
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
	hooks, err := r.loadHooks()
	if err != nil {
		return err
	}

	var targetHook *hookConfig
	for _, h := range hooks {
		domH := hook.FromRepository(h.ID, h.Name, h.Description, h.On.ImageDispatch != nil, h.On.NoteDispatch != nil, nil, false, false)
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

	var events []hookEvent
	for i, id := range ids {
		var path string
		if i < len(paths) {
			path = paths[i]
		}

		events = append(events, hookEvent{
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

	// 从第一个路径推导目录信息，触发必然涉及图片，目录信息总是可推导的
	dir, err := r.resolveDirFromPath(ctx, paths[0])
	if err != nil {
		return fmt.Errorf("failed to resolve directory for trigger %q: %w", triggerName, err)
	}

	_, _, _, err = r.executeHookSync(*targetHook, triggerName, events, nil, "", dir, "", "")
	return err
}

// TriggerForNote 手动派发笔记触发的外部钩子任务
func (r *Runner) TriggerForNote(ctx context.Context, noteRelPath string, hookID scalar.ID) error {
	r.logger.Debug("TriggerForNote start", zap.String("noteRelPath", noteRelPath), zap.String("hookID", hookID.String()))
	hooks, err := r.loadHooks()
	if err != nil {
		return err
	}

	var targetHook *hookConfig
	for _, h := range hooks {
		domH := hook.FromRepository(h.ID, h.Name, h.Description, h.On.ImageDispatch != nil, h.On.NoteDispatch != nil, nil, false, false)
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

	// 依照领域仓库通过相对路径加载目录实体
	dir, err := r.dirRepo.Get(ctx, dirRelPath)
	if err != nil {
		return fmt.Errorf("failed to get directory for note dispatch: %w", err)
	}

	if targetHook.Directive != nil && targetHook.Directive.Name != "" {
		r.logger.Debug("TriggerForNote directive matches, will execute directives", zap.String("directiveName", targetHook.Directive.Name))
		noteAbsPath := filepath.Join(r.rootDir, noteRelPath)
		contentBytes, err := os.ReadFile(noteAbsPath)
		if err != nil {
			return fmt.Errorf("failed to read note file for dispatch: %w", err)
		}
		content := string(contentBytes)

		executed, err := r.executeNoteDirectives(ctx, dir, noteRelPath, content, "note_dispatch", hookID)
		r.logger.Debug("TriggerForNote executeNoteDirectives finished", zap.Bool("executed", executed), zap.Error(err))
		if err != nil {
			return err
		}

		if !executed {
			r.logger.Debug("TriggerForNote not executed by directives, fallback to executeHookSync", zap.String("hookID", targetHook.ID))
			_, _, _, err = r.executeHookSync(*targetHook, "note_dispatch", events, nil, noteRelPath, dir, "", "")
			return err
		}
		return nil
	}

	r.logger.Debug("TriggerForNote no directive defined, executing hook directly", zap.String("hookID", targetHook.ID))
	_, _, _, err = r.executeHookSync(*targetHook, "note_dispatch", events, nil, noteRelPath, dir, "", "")
	return err
}

// runListener 异步监听 EventBus 发来的元数据修改事件以及文件变更事件
func (r *Runner) runListener(ctx context.Context) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		for event, err := range r.fileChangedSub.Subscribe(ctx) {
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				continue
			}
			if err := r.handleFileChanged(event); err != nil {
				r.logger.Error("failed to handle file changed event", zap.Error(err))
			}
		}
	}()

	for event, err := range r.metadataUpdatedSub.Subscribe(ctx) {
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}

		r.handleMetadataUpdated(event)
	}
}

func (r *Runner) onDebounceTrigger(hookID string, events []hookEvent) {
	hooks, err := r.loadHooks()
	if err != nil {
		r.logger.Error("failed to load hooks on debounce callback", zap.Error(err))
		return
	}

	var targetHook *hookConfig
	for _, h := range hooks {
		if h.ID == hookID {
			targetHook = &h
			break
		}
	}

	if targetHook == nil {
		return
	}

	// 从第一个事件中推导目录信息，触发必然涉及图片，目录信息总是可推导的
	dir, err := r.resolveDirFromPath(r.ctx, events[0].Path)
	if err != nil {
		r.logger.Error("failed to resolve directory for hook event, skipping",
			zap.String("hook_id", targetHook.ID),
			zap.String("trigger", "post_update_image_metadata"),
			zap.String("event_path", events[0].Path),
			zap.Error(err),
		)
		return
	}

	r.ch <- hookExecutionTask{
		HookID:      targetHook.ID,
		HookName:    targetHook.Name,
		Command:     targetHook.Command,
		TriggerName: "post_update_image_metadata",
		Events:      events,
		Env:         targetHook.Env,
		resultChan:  make(chan hookExecutionResult, 1),
		dir:         dir,
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

// dirRelFromAbsPath 从绝对路径中提取相对于 rootDir 的目录路径
func (r *Runner) dirRelFromAbsPath(absPath string) string {
	relPath, err := filepath.Rel(r.rootDir, absPath)
	if err != nil {
		return ""
	}
	return filepath.Dir(relPath)
}

// resolveDirFromPath 从绝对路径中推导目录实体
func (r *Runner) resolveDirFromPath(ctx context.Context, absPath string) (*directory.Directory, error) {
	dirRel := r.dirRelFromAbsPath(absPath)
	dir, err := r.dirRepo.Get(ctx, dirRel)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve directory for %q: %w", dirRel, err)
	}
	return dir, nil
}
