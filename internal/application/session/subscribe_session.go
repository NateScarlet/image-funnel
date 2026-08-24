package session

import (
	"context"
	"iter"
	"main/internal/shared"
)

func (h *Handler) SubscribeSession(ctx context.Context) iter.Seq2[*shared.SessionDTO, error] {
	return func(yield func(*shared.SessionDTO, error) bool) {
		for id, err := range h.sessionSavedSub.Subscribe(ctx) {
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			sess, release, err := h.sessionService.Acquire(ctx, id)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			dto, err := h.dtoFactory.New(sess)
			release()
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			if !yield(dto, nil) {
				return
			}
		}
	}
}
