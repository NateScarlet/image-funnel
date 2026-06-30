package session

import (
	"context"
	"fmt"
	"main/internal/scalar"
	"main/internal/shared"
	"time"

	"go.uber.org/zap"
)

func (h *Handler) Commit(
	ctx context.Context,
	sessionID scalar.ID,
	keepRating *int,
	shelveRating *int,
	rejectRating *int,
) (written int, matched int, err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("commit session failed",
				zap.Stringer("sessionID", sessionID),
				zap.Duration("duration", time.Since(startTime)),
				zap.Int("written", written),
				zap.Int("matched", matched),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did commit session",
				zap.Stringer("sessionID", sessionID),
				zap.Duration("duration", time.Since(startTime)),
				zap.Int("written", written),
				zap.Int("matched", matched),
			)
		}
	}()

	sess, release, err := h.sessionService.Acquire(ctx, sessionID)
	if err != nil {
		return 0, 0, fmt.Errorf("session not found: %w", err)
	}
	defer release()

	writeActions := &shared.WriteActions{
		KeepRating:   keepRating,
		ShelveRating: shelveRating,
		RejectRating: rejectRating,
	}
	written, matched, err = h.sessionService.Commit(ctx, sess, writeActions)
	if err != nil {
		return 0, 0, err
	}



	return written, matched, nil
}