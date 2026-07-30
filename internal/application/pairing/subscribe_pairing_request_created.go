package pairing

import (
	"context"
	"iter"
	"main/internal/shared"
)

func (h *Handler) SubscribePairingRequestCreated(ctx context.Context) iter.Seq2[*shared.PairingRequestDTO, error] {
	return func(yield func(*shared.PairingRequestDTO, error) bool) {
		for req, err := range h.pairingSvc.SubscribeRequestCreated(ctx) {
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			dto := h.dtoFactory.New(req, shared.PairingRequestStatusPending)
			if !yield(dto, nil) {
				return
			}
		}
	}
}
