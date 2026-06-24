package note

import (
	"context"
	"iter"
	"main/internal/scalar"
	"main/internal/shared"
	"strings"
)

// NoteUpdated 订阅特定笔记更新
func (h *Handler) NoteUpdated(ctx context.Context, id scalar.ID) iter.Seq2[*shared.NoteDTO, error] {
	return func(yield func(*shared.NoteDTO, error) bool) {
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

			n, err := h.repo.Read(ctx, event.RelPath)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			if n == nil || n.ID() != id {
				continue
			}

			if !yield(h.dtoFactory.New(n), nil) {
				return
			}
		}
	}
}
