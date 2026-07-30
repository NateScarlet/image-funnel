package device

import (
	"context"
	"iter"
	"main/internal/shared"
)

func (h *Handler) SubscribeSaved(ctx context.Context) iter.Seq2[*shared.DeviceDTO, error] {
	return func(yield func(*shared.DeviceDTO, error) bool) {
		for dev, err := range h.deviceSavedSub.Subscribe(ctx) {
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			dto := h.dtoFactory.New(dev)
			if !yield(dto, nil) {
				return
			}
		}
	}
}
