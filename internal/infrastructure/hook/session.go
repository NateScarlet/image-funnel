package hook

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"main/internal/scalar"
	"main/internal/util"

	"go.uber.org/zap"
)

func (r *Runner) OnCommitSession(ctx context.Context, dirRelPath string) error {
	// 触发异步后台任务，尽快返回给调用者
	go func() {
		logErr := func(msg string, err error) {
			if err != nil {
				r.logger.Error(msg, zap.Error(err))
			}
		}

		dir, err := r.dirRepo.Get(r.ctx, dirRelPath)
		if err != nil {
			logErr("failed to get directory for post_commit_session", err)
			return
		}

		hooks, err := r.loadHooks()
		if err != nil {
			logErr("failed to load hooks for post_commit_session in background", err)
			return
		}

		// 1. 触发纯会话提交钩子
		var pureCommitHooks []hookConfig
		for _, h := range hooks {
			if h.On.PostCommitSession != nil && h.On.PostCommitSession.NoteScan == nil {
				pureCommitHooks = append(pureCommitHooks, h)
			}
		}

		pureErrB := util.NewErrorsBuilder(len(pureCommitHooks))
		for _, h := range pureCommitHooks {
			if _, _, _, err := r.executeHookSync(h, "post_commit_session", nil, nil, "", dir, ""); err != nil {
				pureErrB.Add(fmt.Errorf("hook %s: %w", h.ID, err))
			}
		}
		logErr("failed to execute pure post_commit_session hooks", pureErrB.Build())

		// 2. 扫描笔记文件并处理
		dirAbsPath := filepath.Join(r.rootDir, dirRelPath)
		entries, err := os.ReadDir(dirAbsPath)
		if err != nil {
			logErr("failed to read directory for post_commit_session note scan", err)
			return
		}

		noteErrB := util.NewErrorsBuilder(len(entries))
		for _, entry := range entries {
			if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".md" {
				continue
			}

			noteRelPath := filepath.ToSlash(filepath.Join(dirRelPath, entry.Name()))
			noteAbsPath := filepath.Join(r.rootDir, noteRelPath)

			contentBytes, err := os.ReadFile(noteAbsPath)
			if err != nil {
				noteErrB.Add(fmt.Errorf("failed to read note %s: %w", noteRelPath, err))
				continue
			}

			// 2a. 带有指令的钩子：解析与执行指令
			_, err = r.executeNoteDirectives(r.ctx, dir, noteRelPath, string(contentBytes), "post_commit_session", scalar.ID{})
			if err != nil {
				noteErrB.Add(fmt.Errorf("failed to process note directives for %s: %w", noteRelPath, err))
				continue
			}

			// 2b. 无指令的 note_scan 钩子
			var noDirectiveNoteScanHooks []hookConfig
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
					noteErrB.Add(fmt.Errorf("failed to get associated image for %s: %w", noteRelPath, err))
				} else {
					scanErrB := util.NewErrorsBuilder(len(noDirectiveNoteScanHooks))
					for _, h := range noDirectiveNoteScanHooks {
						if _, _, _, hookErr := r.executeHookSync(h, "post_commit_session", evs, nil, noteRelPath, dir, ""); hookErr != nil {
							scanErrB.Add(fmt.Errorf("hook %s: %w", h.ID, hookErr))
						}
					}
					noteErrB.Add(scanErrB.Build())
				}
			}
		}
		logErr("failed to process notes during commit scan", noteErrB.Build())
	}()

	// 立即返回，不等待后台任务完成
	return nil
}