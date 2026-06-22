package pairing

import "context"

func (h *Handler) ApprovePairingRequest(ctx context.Context, code string) error {
	return h.deviceService.ApproveRequest(ctx, code)
}