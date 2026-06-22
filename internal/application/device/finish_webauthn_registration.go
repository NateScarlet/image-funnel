package device

import (
	"context"
	"main/internal/shared"
	"time"

	"go.uber.org/zap"
)

func (h *Handler) FinishWebAuthnRegistration(ctx context.Context, sessionKey string, responseJSON string, setupToken string) (device *shared.DeviceDTO, pr *shared.PairingRequestDTO, accessTokenRef string, refreshTokenRef string, accessExpiresAt time.Time, refreshExpiresAt time.Time, err error) {
	startTime := time.Now()
	defer func() {
		if err != nil {
			h.logger.Error("finish webauthn registration failed",
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did finish webauthn registration",
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	dev, pairingReq, err := h.service.FinishRegistration(ctx, sessionKey, responseJSON, setupToken, IsTrustedIP(ctx), RemoteIP(ctx), UserAgent(ctx))
	if err != nil {
		return nil, nil, "", "", time.Time{}, time.Time{}, err
	}

	var deviceDTO *shared.DeviceDTO
	var accessExp, refreshExp time.Time
	if dev != nil {
		accessTokenRef, refreshTokenRef, accessExp, refreshExp, err = h.GenerateToken(ctx, dev.ID())
		if err != nil {
			return nil, nil, "", "", time.Time{}, time.Time{}, err
		}
		deviceDTO = h.dtoFactory.New(dev)
	}

	var prDTO *shared.PairingRequestDTO
	if pairingReq != nil {
		prDTO = &shared.PairingRequestDTO{
			Code:      pairingReq.Code(),
			CreatedAt: pairingReq.CreatedAt(),
			Status:    shared.PairingRequestStatusPending,
		}
	}

	return deviceDTO, prDTO, accessTokenRef, refreshTokenRef, accessExp, refreshExp, nil
}