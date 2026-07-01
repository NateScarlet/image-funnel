package hook

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"main/internal/scalar"

	"go.uber.org/zap"
)

func (r *Runner) OnCommitSession(ctx context.Context, dirID scalar.ID, dirRelPath string) error {
	// 触发异步后台任务，尽快返回给调用者
	go func() {
		hooks, err := r.loadHooks()
		if err != nil {
			r.logger.Error("failed to load hooks for post_commit_session in background", zap.Error(err))
			return
		}

		// 1. 触发纯会话提交钩子 (配置了 post_commit_session 但没有配置 note_scan 属性的钩子)
		var pureCommitHooks []hookConfig
		for _, h := range hooks {
			if h.On.PostCommitSession != nil && h.On.PostCommitSession.NoteScan == nil {
				pureCommitHooks = append(pureCommitHooks, h)
			}
		}

		if len(pureCommitHooks) > 0 {
			// 彻底禁止为了钩子加载目录下所有图片！仅传入空列表和会话目录信息，由脚本端自行 GraphQL 按需过滤
			for _, h := range pureCommitHooks {
				_, _, _, err = r.executeHookSync(h, "post_commit_session", nil, nil, "", dirID.String(), dirRelPath, "")
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
					r.logger.Error("failed to get associated image for commit scan hook", zap.String("note_path", noteRelPath), zap.Error(err))
				}
				for _, h := range noDirectiveNoteScanHooks {
					_, _, _, err = r.executeHookSync(h, "post_commit_session", evs, nil, noteRelPath, dirID.String(), dirRelPath, "")
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