package note

import (
	"context"
	"iter"
	"main/internal/domain/relpath"
	"main/internal/shared"
	"strings"
)

// NoteSaved 订阅笔记保存完成事件（支持目录/ID/是否隐藏等条件灵活过滤）。
// 只消费服务端在全部处理完成后发布的事件，避免读取正在被写入的笔记文件。
func (h *Handler) NoteSaved(ctx context.Context, filter *shared.NoteFilters) iter.Seq2[*shared.NoteDTO, error] {
	return func(yield func(*shared.NoteDTO, error) bool) {
		var filters shared.NoteFilters
		if filter != nil {
			filters = *filter
		}

		// 目录条件解析为允许的根相对路径，按笔记所在目录直接比对
		var allowedDirs []relpath.Path
		for _, id := range filters.DirectoryID {
			dir, err := h.dirSvc.GetDirectory(ctx, id)
			if err != nil {
				yield(nil, err)
				return
			}
			allowedDirs = append(allowedDirs, dir.RelPath())
		}

		noteFilter := h.filterBuilder.Build(filters)

		for event, err := range h.savedEvents.Subscribe(ctx) {
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			if !strings.HasSuffix(strings.ToLower(event.RelPath.String()), ".md") {
				continue
			}

			if filters.DirectoryID != nil && !dirAllowed(event.RelPath.Dir(), allowedDirs) {
				continue
			}

			n, err := h.repo.Read(ctx, event.RelPath)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			if n == nil || !noteFilter(n) {
				continue
			}

			if !yield(h.dtoFactory.New(n), nil) {
				return
			}
		}
	}
}

// dirAllowed 判断笔记所在目录是否落在允许的目录集合中
func dirAllowed(noteDir relpath.Path, allowedDirs []relpath.Path) bool {
	for _, dir := range allowedDirs {
		if noteDir == dir {
			return true
		}
	}
	return false
}
