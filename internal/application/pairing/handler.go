package pairing

import (
	"main/internal/domain/device"
	dompairing "main/internal/domain/pairing"

	"go.uber.org/zap"
)

type Handler struct {
	deviceService *device.Service
	pairingSvc    *dompairing.Service
	dtoFactory    *DTOFactory
	logger        *zap.Logger
}

func NewHandler(logger *zap.Logger, deviceService *device.Service, pairingSvc *dompairing.Service, dtoFactory *DTOFactory) *Handler {
	return &Handler{
		logger:        logger,
		deviceService: deviceService,
		pairingSvc:    pairingSvc,
		dtoFactory:    dtoFactory,
	}
}