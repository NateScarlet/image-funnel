package pairing

import "context"

func (h *Handler) RejectPairingRequest(ctx context.Context, code string) error {
	return h.deviceService.RejectRequest(ctx, code)
}