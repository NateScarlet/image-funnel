package pairing

import (
	"context"
	"iter"
	"main/internal/shared"
)

func (h *Handler) SubscribePairingRequestCreated(ctx context.Context) iter.Seq2[*shared.PairingRequestDTO, error] {
	return h.pairingSvc.SubscribeRequestCreated(ctx)
}