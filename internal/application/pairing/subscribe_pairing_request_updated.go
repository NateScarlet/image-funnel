package pairing

import (
	"context"
	"iter"
	"main/internal/shared"
)

func (h *Handler) SubscribePairingRequestUpdated(ctx context.Context) iter.Seq2[*shared.PairingRequestDTO, error] {
	return func(yield func(*shared.PairingRequestDTO, error) bool) {
		for event, err := range h.pairingSvc.SubscribeRequestResolved(ctx) {
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			dto := h.dtoFactory.New(event.Request, event.Status)
			if !yield(dto, nil) {
				return
			}
		}
	}
}
