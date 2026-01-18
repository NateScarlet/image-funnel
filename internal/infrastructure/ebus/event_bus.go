package ebus

import (
	"context"
	"iter"
	"main/internal/application/session"
	"main/internal/pubsub"
)

type EventBus struct {
	Session pubsub.Topic[*session.SessionDTO]
}

func NewEventBus(sessionTopic pubsub.Topic[*session.SessionDTO]) *EventBus {
	return &EventBus{
		Session: sessionTopic,
	}
}

func (b *EventBus) PublishSession(ctx context.Context, sessionDTO *session.SessionDTO) {
	b.Session.Publish(ctx, sessionDTO)
}

func (b *EventBus) SubscribeSession(ctx context.Context) iter.Seq2[*session.SessionDTO, error] {
	return b.Session.Subscribe(ctx)
}

var _ session.EventBus = (*EventBus)(nil)
