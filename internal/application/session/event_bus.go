package session

import (
	"context"
	"iter"
	"main/internal/shared"
)

type EventBus interface {
	PublishFileChanged(ctx context.Context, event *shared.FileChangedEvent)
	SubscribeFileChanged(ctx context.Context) iter.Seq2[*shared.FileChangedEvent, error]
	SubscribeSession(ctx context.Context) iter.Seq2[*shared.SessionDTO, error]
}
