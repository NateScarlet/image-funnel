package memo

import (
	"context"
	"iter"
	"main/internal/scalar"
	"main/internal/shared"
	"main/internal/util"
	"strings"
)

// MemoSaved 订阅备忘录改变（新接口，支持目录/ID/是否隐藏等条件灵活过滤）
func (h *Handler) MemoSaved(ctx context.Context, filter *shared.MemoFilters) iter.Seq2[*shared.MemoDTO, error] {
	return func(yield func(*shared.MemoDTO, error) bool) {
		var filters shared.MemoFilters
		if filter != nil {
			filters = *filter
		}

		var allowedDirectoryIDs util.Set[scalar.ID]
		if filters.DirectoryID != nil {
			allowedDirectoryIDs = util.AddToSet(nil, filters.DirectoryID...)
		}

		memoFilter := h.filterBuilder.Build(filters)

		for event, err := range h.ebus.SubscribeFileChanged(ctx) {
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

			m, err := h.repo.Read(ctx, event.RelPath)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			if m == nil || !memoFilter(m) {
				continue
			}

			if !yield(h.dtoFactory.New(m), nil) {
				return
			}
		}
	}
}