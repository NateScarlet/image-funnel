package note

import (
	"context"
	"iter"
	"main/internal/scalar"
	"main/internal/shared"
	"main/internal/util"
	"strings"
)

// NoteSaved 订阅笔记改变（新接口，支持目录/ID/是否隐藏等条件灵活过滤）
func (h *Handler) NoteSaved(ctx context.Context, filter *shared.NoteFilters) iter.Seq2[*shared.NoteDTO, error] {
	return func(yield func(*shared.NoteDTO, error) bool) {
		var filters shared.NoteFilters
		if filter != nil {
			filters = *filter
		}

		var allowedDirectoryIDs util.Set[scalar.ID]
		if filters.DirectoryID != nil {
			allowedDirectoryIDs = util.AddToSet(nil, filters.DirectoryID...)
		}

		noteFilter := h.filterBuilder.Build(filters)

		for event, err := range h.fileChangedSub.Subscribe(ctx) {
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			if !strings.HasSuffix(strings.ToLower(event.RelPath), ".md") {
				continue
			}

			if allowedDirectoryIDs != nil && !allowedDirectoryIDs.Has(event.DirectoryID) {
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
