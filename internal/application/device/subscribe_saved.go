package device

import (
	"context"
	"iter"
	"main/internal/shared"
)

func (h *Handler) SubscribeSaved(ctx context.Context) iter.Seq2[*shared.DeviceDTO, error] {
	if h.ebus != nil {
		return h.ebus.SubscribeDeviceSaved(ctx)
	}
	return func(yield func(*shared.DeviceDTO, error) bool) {}
}