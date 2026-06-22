package session

import (
	"context"
	"main/internal/scalar"
	"main/internal/shared"
)

func (h *Handler) Session(ctx context.Context, sessionID scalar.ID) (*shared.SessionDTO, error) {
	sess, release, err := h.sessionService.Acquire(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	defer release()

	return h.dtoFactory.New(sess)
}