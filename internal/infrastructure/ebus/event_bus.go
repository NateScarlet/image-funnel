package ebus

import (
	"context"
	"iter"
	session "main/internal/application/session"
	"main/internal/pubsub"
	"main/internal/shared"
)

type EventBus struct {
	Session pubsub.Topic[*shared.SessionDTO]
}

func NewEventBus(sessionTopic pubsub.Topic[*shared.SessionDTO]) *EventBus {
	return &EventBus{
		Session: sessionTopic,
	}
}

func (b *EventBus) PublishSession(ctx context.Context, sessionDTO *shared.SessionDTO) {
	b.Session.Publish(ctx, sessionDTO)
}

func (b *EventBus) SubscribeSession(ctx context.Context) iter.Seq2[*shared.SessionDTO, error] {
	return b.Session.Subscribe(ctx)
}

var _ session.EventBus = (*EventBus)(nil)
