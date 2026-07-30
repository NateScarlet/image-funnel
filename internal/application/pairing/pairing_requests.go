package pairing

import (
	"context"
	"main/internal/shared"
)

func (h *Handler) PairingRequests(ctx context.Context) (dtos []*shared.PairingRequestDTO, err error) {
	var results []*shared.PairingRequestDTO
	for pr, err := range h.pairingSvc.Find(ctx) {
		if err != nil {
			return nil, err
		}
		results = append(results, h.dtoFactory.New(pr, shared.PairingRequestStatusPending))
	}
	return results, nil
}
