package device

import (
	"context"
	"iter"
	"main/internal/scalar"
)

func (h *Handler) SubscribeDeleted(ctx context.Context) iter.Seq2[scalar.ID, error] {
	if h.deviceDeletedSub != nil {
		return h.deviceDeletedSub.Subscribe(ctx)
	}
	return func(yield func(scalar.ID, error) bool) {}
}