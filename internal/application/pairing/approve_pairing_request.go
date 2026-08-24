package pairing

import (
	"context"
	"time"

	"go.uber.org/zap"
)

func (h *Handler) ApprovePairingRequest(ctx context.Context, code string) (err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("approve pairing request failed",
				zap.String("code", code),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did approve pairing request",
				zap.String("code", code),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	return h.deviceService.ApproveRequest(ctx, code)
}
