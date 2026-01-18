package session

import (
	"context"
	"iter"
)

type EventBus interface {
	PublishSession(ctx context.Context, sessionDTO *SessionDTO)
	SubscribeSession(ctx context.Context) iter.Seq2[*SessionDTO, error]
}
