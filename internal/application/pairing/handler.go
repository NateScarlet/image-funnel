package pairing

import (
	"main/internal/domain/device"
	dompairing "main/internal/domain/pairing"
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