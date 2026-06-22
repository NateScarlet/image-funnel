package device

import "context"

func (h *Handler) AuthStatus(ctx context.Context) (*AuthStatusDTO, error) {
	isTrustedDevice := IsTrustedDevice(ctx)
	isTrustedIP := IsTrustedIP(ctx)

	return &AuthStatusDTO{
		IsTrustedDevice: isTrustedDevice,
		IsTrustedIP:     isTrustedIP,
		CanAccess:       isTrustedDevice || isTrustedIP,
	}, nil
}

type AuthStatusDTO struct {
	IsTrustedDevice bool
	IsTrustedIP     bool
	CanAccess       bool
}