package hook

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"main/internal/domain/relpath"
	"main/internal/util"

	"go.uber.org/zap"
)

// sessionCommitTrigger 会话提交触发路径（post_commit_session）：
// 先触发纯提交钩子，再扫描目录中的笔记，依次处理斜杠指令与无指令的 note_scan 钩子。
// 整条路径在后台异步执行，不阻塞会话提交的返回。
type sessionCommitTrigger struct {
	bgCtx   context.Context
	rootDir string
	config  configSource
	exec    *hookExecutor
	rewrite *noteRewriter
	notes   *noteContext
	logger  *zap.Logger
}

func newSessionCommitTrigger(
	bgCtx context.Context,
	rootDir string,
	config configSource,
	exec *hookExecutor,
	rewrite *noteRewriter,
	notes *noteContext,
	logger *zap.Logger,
) *sessionCommitTrigger {
	return &sessionCommitTrigger{
		bgCtx:   bgCtx,
		rootDir: rootDir,
		config:  config,
		exec:    exec,
		rewrite: rewrite,
		notes:   notes,
		logger:  logger,
	}
}

// Commit 触发提交后的钩子与笔记扫描后立即返回，后台任务的生命周期随 Runner 结束而取消
func (t *sessionCommitTrigger) Commit(dirRelPath relpath.Path) {
	go t.run(t.bgCtx, dirRelPath)
}

func (t *sessionCommitTrigger) run(ctx context.Context, dirRelPath relpath.Path) {
	dir, err := t.notes.DirOfRelPath(ctx, dirRelPath)
	if err != nil {
		t.logger.Error("failed to get directory for post_commit_session", zap.Error(err))
		return
	}

	hooks, err := t.config.Load()
	if err != nil {
		t.logger.Error("failed to load hooks for post_commit_session in background", zap.Error(err))
		return
	}

	// 1. 触发纯会话提交钩子
	var pureCommitHooks []hookConfig
	for _, h := range hooks {
		if h.On.PostCommitSession != nil && h.On.PostCommitSession.NoteScan == nil {
			pureCommitHooks = append(pureCommitHooks, h)
		}
	}
	slices.SortStableFunc(pureCommitHooks, sortByOrderAndFilename)

	pureErrB := util.NewErrorsBuilder(len(pureCommitHooks))
	for _, h := range pureCommitHooks {
		if res := t.exec.Execute(newHookTask(h, "post_commit_session", nil, "", dir)); res.Err != nil {
			pureErrB.Add(fmt.Errorf("hook %s: %w", h.ID, res.Err))
		}
	}
	if err := pureErrB.Build(); err != nil {
		t.logger.Error("failed to execute pure post_commit_session hooks", zap.Error(err))
	}

	// 2. 扫描笔记文件并处理
	noDirectiveNoteScanHooks := make([]hookConfig, 0, len(hooks))
	for _, h := range hooks {
		if h.On.PostCommitSession != nil && h.On.PostCommitSession.NoteScan != nil {
			if h.Directive == nil || h.On.PostCommitSession.NoteScan.IgnoreDirective {
				noDirectiveNoteScanHooks = append(noDirectiveNoteScanHooks, h)
			}
		}
	}
	slices.SortStableFunc(noDirectiveNoteScanHooks, sortByOrderAndFilename)

	entries, err := os.ReadDir(dirRelPath.Abs(t.rootDir))
	if err != nil {
		t.logger.Error("failed to read directory for post_commit_session note scan", zap.Error(err))
		return
	}

	noteErrB := util.NewErrorsBuilder(len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".md" {
			continue
		}

		// 笔记项来自文件系统遍历，在此处构造根相对路径（出生点之一）并围栏
		noteRelPath, err := dirRelPath.Join(t.rootDir, entry.Name())
		if err != nil {
			noteErrB.Add(fmt.Errorf("failed to resolve note path for %s: %w", entry.Name(), err))
			continue
		}

		contentBytes, err := os.ReadFile(noteRelPath.Abs(t.rootDir))
		if err != nil {
			noteErrB.Add(fmt.Errorf("failed to read note %s: %w", noteRelPath, err))
			continue
		}

		// 2a. 带有指令的钩子：解析与执行指令
		_, err = t.rewrite.Rewrite(ctx, noteRewriteRequest{
			Dir:         dir,
			NoteRelPath: noteRelPath,
			Content:     string(contentBytes),
			TriggerName: "post_commit_session",
		})
		if err != nil {
			noteErrB.Add(fmt.Errorf("failed to process note directives for %s: %w", noteRelPath, err))
			continue
		}

		// 2b. 无指令的 note_scan 钩子
		if len(noDirectiveNoteScanHooks) == 0 {
			continue
		}

		evs, err := t.notes.AssociatedImageEvents(ctx, noteRelPath)
		if err != nil {
			noteErrB.Add(fmt.Errorf("failed to get associated image for %s: %w", noteRelPath, err))
			continue
		}

		scanErrB := util.NewErrorsBuilder(len(noDirectiveNoteScanHooks))
		for _, h := range noDirectiveNoteScanHooks {
			if res := t.exec.Execute(newHookTask(h, "post_commit_session", evs, noteRelPath.String(), dir)); res.Err != nil {
				scanErrB.Add(fmt.Errorf("hook %s: %w", h.ID, res.Err))
			}
		}
		noteErrB.Add(scanErrB.Build())
	}
	if err := noteErrB.Build(); err != nil {
		t.logger.Error("failed to process notes during commit scan", zap.Error(err))
	}
}
