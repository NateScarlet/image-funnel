package pairing

import (
	"context"
	"main/internal/shared"
)

func (h *Handler) PairingRequest(ctx context.Context, code string) *shared.PairingRequestDTO {
	pr := h.pairingSvc.Get(ctx, code)
	if pr == nil {
		return nil
	}
	return h.dtoFactory.New(pr, shared.PairingRequestStatusPending)
}