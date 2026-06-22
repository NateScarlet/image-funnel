package device

import (
	"context"
	"main/internal/shared"
	"time"

	"go.uber.org/zap"
)

func (h *Handler) FinishWebAuthnLogin(ctx context.Context, sessionKey string, responseJSON string, ip string) (accessTokenRef string, refreshTokenRef string, accessExpiresAt time.Time, refreshExpiresAt time.Time, device *shared.DeviceDTO, err error) {
	startTime := time.Now()
	defer func() {
		if err != nil {
			h.logger.Error("finish webauthn login failed",
				zap.Duration("duration", time.Since(startTime)),
				zap.String("ip", ip),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did finish webauthn login",
				zap.Duration("duration", time.Since(startTime)),
				zap.String("ip", ip),
			)
		}
	}()

	dev, err := h.service.FinishLogin(ctx, sessionKey, responseJSON, ip, UserAgent(ctx))
	if err != nil {
		return "", "", time.Time{}, time.Time{}, nil, err
	}
	accessTokenRef, refreshTokenRef, accessExpiresAt, refreshExpiresAt, err = h.GenerateToken(ctx, dev.ID())
	if err != nil {
		return "", "", time.Time{}, time.Time{}, nil, err
	}
	device = h.dtoFactory.New(dev)
	return accessTokenRef, refreshTokenRef, accessExpiresAt, refreshExpiresAt, device, nil
}