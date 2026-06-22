package pairing

import (
	"context"
	"main/internal/shared"
)

func (h *Handler) PairingRequests(ctx context.Context) ([]*shared.PairingRequestDTO, error) {
	var dtos []*shared.PairingRequestDTO
	for pr, err := range h.pairingSvc.Find(ctx) {
		if err != nil {
			return nil, err
		}
		dtos = append(dtos, h.dtoFactory.New(pr, shared.PairingRequestStatusPending))
	}
	return dtos, nil
}