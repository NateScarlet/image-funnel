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
	keepRating int,
	shelveRating int,
	rejectRating int,
) (success int, err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("commit session failed",
				zap.Stringer("sessionID", sessionID),
				zap.Duration("duration", time.Since(startTime)),
				zap.Int("success", success),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did commit session",
				zap.Stringer("sessionID", sessionID),
				zap.Duration("duration", time.Since(startTime)),
				zap.Int("success", success),
			)
		}
	}()

	sess, release, err := h.sessionService.Acquire(ctx, sessionID)
	if err != nil {
		return 0, fmt.Errorf("session not found: %w", err)
	}
	defer release()

	writeActions := &shared.WriteActions{
		KeepRating:   keepRating,
		ShelveRating: shelveRating,
		RejectRating: rejectRating,
	}
	successCount, err := h.sessionService.Commit(ctx, sess, writeActions)
	if err != nil {
		return 0, err
	}

	if h.lastSessionSaver != nil {
		_ = h.lastSessionSaver.SaveLastSessionCommitActions(ctx, sess.DirectoryID(), sessionID, writeActions)
	}

	return successCount, nil
}