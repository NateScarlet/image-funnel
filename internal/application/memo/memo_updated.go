package memo

import (
	"context"
	"iter"
	"main/internal/scalar"
	"main/internal/shared"
	"strings"
)

// MemoUpdated 订阅特定备忘录更新（保持老版本向后兼容）
func (h *Handler) MemoUpdated(ctx context.Context, id scalar.ID) iter.Seq2[*shared.MemoDTO, error] {
	return func(yield func(*shared.MemoDTO, error) bool) {
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

			m, err := h.repo.Read(ctx, event.RelPath)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			if m == nil || m.ID() != id {
				continue
			}

			if !yield(h.dtoFactory.New(m), nil) {
				return
			}
		}
	}
}