package session

import (
	"context"
	"iter"
	"main/internal/shared"
)

type EventBus interface {
	PublishSession(ctx context.Context, sessionDTO *shared.SessionDTO)
	SubscribeSession(ctx context.Context) iter.Seq2[*shared.SessionDTO, error]
}
