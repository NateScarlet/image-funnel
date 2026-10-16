package hook

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"main/internal/domain/directory"
	"main/internal/shared"
	"main/internal/util"

	"go.uber.org/zap"
)

// noteUpdateTrigger 笔记文件变更触发路径（post_update_note）：
// 先让指令改写器处理斜杠指令，再触发声明了「无指令」的笔记钩子。
type noteUpdateTrigger struct {
	bgCtx   context.Context
	rootDir string
	config  configSource
	exec    *hookExecutor
	rewrite *noteRewriter
	guard   *writeGuard
	notes   *noteContext
	logger  *zap.Logger
}

func newNoteUpdateTrigger(
	bgCtx context.Context,
	rootDir string,
	config configSource,
	exec *hookExecutor,
	rewrite *noteRewriter,
	guard *writeGuard,
	notes *noteContext,
	logger *zap.Logger,
) *noteUpdateTrigger {
	return &noteUpdateTrigger{
		bgCtx:   bgCtx,
		rootDir: rootDir,
		config:  config,
		exec:    exec,
		rewrite: rewrite,
		guard:   guard,
		notes:   notes,
		logger:  logger,
	}
}

// HandleFileChanged 处理一次笔记文件变更；非 .md、删除事件与自身写入回环一律忽略
func (t *noteUpdateTrigger) HandleFileChanged(event *directory.FileChangedEvent) error {
	if event.Action != shared.FileActionCreate && event.Action != shared.FileActionWrite {
		return nil
	}
	if strings.ToLower(filepath.Ext(event.RelPath.String())) != ".md" {
		return nil
	}

	noteAbsPath := event.RelPath.Abs(t.rootDir)
	contentBytes, err := os.ReadFile(noteAbsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read note file for directive processing: %w", err)
	}

	// 检查内容哈希防重入列表，若是自身写入触发的事件则直接忽略
	if t.guard.ShouldIgnore(noteAbsPath, contentBytes) {
		t.logger.Debug("ignoring change event as it was triggered by our own write", zap.String("path", event.RelPath.String()))
		return nil
	}

	dir, err := t.notes.DirOfRelPath(t.bgCtx, event.RelPath.Dir())
	if err != nil {
		return fmt.Errorf("failed to get directory for note update: %w", err)
	}

	// 1. 处理包含指令的钩子：执行指令并回写文本
	_, err = t.rewrite.Rewrite(t.bgCtx, noteRewriteRequest{
		Dir:         dir,
		NoteRelPath: event.RelPath,
		Content:     string(contentBytes),
		TriggerName: "post_update_note",
	})
	if err != nil {
		return fmt.Errorf("failed to process note directives: %w", err)
	}

	// 2. 触发无指令要求的笔记修改钩子
	hooks, err := t.config.Load()
	if err != nil {
		return fmt.Errorf("failed to load hooks: %w", err)
	}

	var noDirectiveHooks []hookConfig
	for _, h := range hooks {
		if h.On.PostUpdateNote == nil {
			continue
		}
		if h.Directive == nil || h.On.PostUpdateNote.IgnoreDirective {
			noDirectiveHooks = append(noDirectiveHooks, h)
		}
	}
	slices.SortStableFunc(noDirectiveHooks, sortByOrderAndFilename)

	if len(noDirectiveHooks) == 0 {
		return nil
	}

	evs, err := t.notes.AssociatedImageEvents(t.bgCtx, event.RelPath)
	if err != nil {
		return fmt.Errorf("failed to get associated image for note update hook: %w", err)
	}

	errB := util.NewErrorsBuilder(len(noDirectiveHooks))
	for _, h := range noDirectiveHooks {
		if res := t.exec.Execute(newHookTask(h, "post_update_note", evs, event.RelPath.String(), dir)); res.Err != nil {
			errB.Add(fmt.Errorf("hook %s: %w", h.ID, res.Err))
		}
	}
	if err := errB.Build(); err != nil {
		return fmt.Errorf("failed to execute non-directive hooks: %w", err)
	}
	return nil
}
