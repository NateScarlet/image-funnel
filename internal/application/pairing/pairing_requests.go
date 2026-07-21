package pairing

import (
	"context"
	"main/internal/shared"
	"time"

	"go.uber.org/zap"
)

func (h *Handler) PairingRequests(ctx context.Context) (dtos []*shared.PairingRequestDTO, err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("pairing requests failed",
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did pairing requests",
				zap.Int("count", len(dtos)),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	var results []*shared.PairingRequestDTO
	for pr, err := range h.pairingSvc.Find(ctx) {
		if err != nil {
			return nil, err
		}
		results = append(results, h.dtoFactory.New(pr, shared.PairingRequestStatusPending))
	}
	return results, nil
}