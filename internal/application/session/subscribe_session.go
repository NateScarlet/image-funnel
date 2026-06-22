package session

import (
	"context"
	"iter"
	"main/internal/shared"
)

func (h *Handler) SubscribeSession(ctx context.Context) iter.Seq2[*shared.SessionDTO, error] {
	return h.eventBus.SubscribeSession(ctx)
}