package device

import (
	"context"
	"iter"
	"main/internal/scalar"
)

func (h *Handler) SubscribeDeleted(ctx context.Context) iter.Seq2[scalar.ID, error] {
	if h.ebus != nil {
		return h.ebus.SubscribeDeviceDeleted(ctx)
	}
	return func(yield func(scalar.ID, error) bool) {}
}