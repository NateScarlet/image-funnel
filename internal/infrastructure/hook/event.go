package hook

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"main/internal/scalar"
	"main/internal/shared"
	"main/internal/util"

	"go.uber.org/zap"
)

func (r *Runner) handleFileChanged(event *shared.FileChangedEvent) error {
	if event.Action != shared.FileActionCreate && event.Action != shared.FileActionWrite {
		return nil
	}
	if strings.ToLower(filepath.Ext(event.RelPath)) != ".md" {
		return nil
	}

	noteAbsPath := filepath.Join(r.rootDir, event.RelPath)
	contentBytes, err := os.ReadFile(noteAbsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read note file for directive processing: %w", err)
	}

	content := string(contentBytes)

	// 0. 检查内容哈希防重入列表，若是自身写入触发的事件则直接忽略
	if r.shouldIgnoreEvent(noteAbsPath, contentBytes) {
		r.logger.Debug("ignoring change event as it was triggered by our own write", zap.String("path", event.RelPath))
		return nil
	}

	dir, err := r.dirRepo.Get(r.ctx, filepath.Dir(event.RelPath))
	if err != nil {
		return fmt.Errorf("failed to get directory for note update: %w", err)
	}

	// 1. 处理包含指令的钩子：执行指令并回写文本
	_, err = r.executeNoteDirectives(r.ctx, dir, event.RelPath, content, "post_update_note", scalar.ID{})
	if err != nil {
		return fmt.Errorf("failed to process note directives: %w", err)
	}

	// 2. 触发无指令要求的笔记修改钩子
	hooks, err := r.loadHooks()
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

	if len(noDirectiveHooks) > 0 {
		evs, err := r.findAssociatedImageEvents(r.ctx, event.RelPath)
		if err != nil {
			return fmt.Errorf("failed to get associated image for note update hook: %w", err)
		}
		errB := util.NewErrorsBuilder(len(noDirectiveHooks))
		for _, h := range noDirectiveHooks {
			if _, _, _, hookErr := r.executeHookSync(h, "post_update_note", evs, nil, event.RelPath, dir, "", ""); hookErr != nil {
				errB.Add(fmt.Errorf("hook %s: %w", h.ID, hookErr))
			}
		}
		if err := errB.Build(); err != nil {
			return fmt.Errorf("failed to execute non-directive hooks: %w", err)
		}
	}
	return nil
}

func (r *Runner) handleMetadataUpdated(event *shared.MetadataUpdatedEvent) {
	hooks, err := r.loadHooks()
	if err != nil {
		r.logger.Error("failed to load hooks during metadata updated event", zap.Error(err))
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
