package session

import (
	"context"
	"main/internal/scalar"
	"main/internal/shared"
)

func (h *Handler) SessionStats(ctx context.Context, sessionID scalar.ID) (*shared.StatsDTO, error) {
	sess, release, err := h.sessionService.Acquire(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	defer release()

	return sess.Stats(), nil
}