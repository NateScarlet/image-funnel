package pairing

import (
	"context"
	"time"

	"go.uber.org/zap"
)

func (h *Handler) RejectPairingRequest(ctx context.Context, code string) (err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("reject pairing request failed",
				zap.String("code", code),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did reject pairing request",
				zap.String("code", code),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	return h.deviceService.RejectRequest(ctx, code)
}