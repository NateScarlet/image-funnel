package device

import (
	"context"
	"iter"
	"main/internal/scalar"
)

func (h *Handler) SubscribeDeleted(ctx context.Context) iter.Seq2[scalar.ID, error] {
	h.logger.Info("will subscribe to device deleted")
	return h.deviceDeletedSub.Subscribe(ctx)
}