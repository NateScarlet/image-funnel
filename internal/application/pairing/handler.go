package pairing

import (
	"context"
	"iter"

	"main/internal/domain/device"
	dompairing "main/internal/domain/pairing"
	"main/internal/shared"
)

type Handler struct {
	deviceService *device.Service
	pairingSvc    *dompairing.Service
	dtoFactory    *DTOFactory
}

func NewHandler(deviceService *device.Service, pairingSvc *dompairing.Service, dtoFactory *DTOFactory) *Handler {
	return &Handler{
		deviceService: deviceService,
		pairingSvc:    pairingSvc,
		dtoFactory:    dtoFactory,
	}
}

func (h *Handler) ApprovePairingRequest(ctx context.Context, code string) error {
	return h.deviceService.ApproveRequest(ctx, code)
}

func (h *Handler) RejectPairingRequest(ctx context.Context, code string) error {
	return h.deviceService.RejectRequest(ctx, code)
}

func (h *Handler) SubscribePairingRequestCreated(ctx context.Context) iter.Seq2[*shared.PairingRequestDTO, error] {
	return h.pairingSvc.SubscribeRequestCreated(ctx)
}

func (h *Handler) SubscribeParingRequestUpdated(ctx context.Context) iter.Seq2[*shared.PairingRequestDTO, error] {
	return h.pairingSvc.SubscribeRequestUpdated(ctx)
}

func (h *Handler) PairingRequest(ctx context.Context, code string) *shared.PairingRequestDTO {
	pr := h.pairingSvc.Get(ctx, code)
	if pr == nil {
		return nil
	}
	return h.dtoFactory.New(pr, shared.PairingRequestStatusPending)
}

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
