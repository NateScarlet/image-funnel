package pairing

import (
	"context"
	"main/internal/shared"
	"time"

	"go.uber.org/zap"
)

func (h *Handler) PairingRequest(ctx context.Context, code string) (dto *shared.PairingRequestDTO) {
	startTime := time.Now()

	defer func() {
		if dto == nil {
			h.logger.Info("did pairing request",
				zap.String("code", code),
				zap.Bool("found", false),
				zap.Duration("duration", time.Since(startTime)),
			)
		} else {
			h.logger.Info("did pairing request",
				zap.String("code", code),
				zap.Bool("found", true),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	pr := h.pairingSvc.Get(ctx, code)
	if pr == nil {
		return nil
	}
	return h.dtoFactory.New(pr, shared.PairingRequestStatusPending)
}