package hook

import (
	"os"
	"path/filepath"
	"strings"

	"main/internal/scalar"
	"main/internal/shared"

	"go.uber.org/zap"
)

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
	hooks, err := r.loadHooks()
	if err != nil {
		return
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

	if len(noDirectiveHooks) > 0 {
		evs, err := r.findAssociatedImageEvents(r.ctx, event.RelPath)
		if err != nil {
			r.logger.Error("failed to get associated image for note update hook", zap.String("note_path", event.RelPath), zap.Error(err))
		}

		for _, h := range noDirectiveHooks {
			_, _, _, err = r.executeHookSync(h, "post_update_note", evs, nil, event.RelPath, event.DirectoryID.String(), dirRelPath, "")
			if err != nil {
				r.logger.Error("failed to execute no-directive post_update_note hook", zap.String("hook_id", h.ID), zap.Error(err))
			}
		}
	}
}

func (r *Runner) handleMetadataUpdated(event *shared.MetadataUpdatedEvent) {
	hooks, err := r.loadHooks()
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
		if !trigger.match(event) {
			continue
		}

		// 加入防抖
		r.debouncer.Add(h.ID, hookEvent{
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