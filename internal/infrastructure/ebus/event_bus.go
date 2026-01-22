package ebus

import (
	"context"
	"iter"

	"main/internal/application/session"
	dsession "main/internal/domain/session"
	"main/internal/pubsub"
	"main/internal/shared"
)

// EventBus 事件总线实现
type EventBus struct {
	sessionTopic     pubsub.Topic[*dsession.Session]
	fileChangedTopic pubsub.Topic[*shared.FileChangedEvent]
	sessionFactory   *session.SessionDTOFactory
}

func NewEventBus(
	sessionTopic pubsub.Topic[*dsession.Session],
	fileChangedTopic pubsub.Topic[*shared.FileChangedEvent],
	sessionFactory *session.SessionDTOFactory,
) *EventBus {
	return &EventBus{
		sessionTopic:     sessionTopic,
		fileChangedTopic: fileChangedTopic,
		sessionFactory:   sessionFactory,
	}
}

func (b *EventBus) SubscribeSession(ctx context.Context) iter.Seq2[*shared.SessionDTO, error] {
	return func(yield func(*shared.SessionDTO, error) bool) {
		for sess, err := range b.sessionTopic.Subscribe(ctx) {
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			if !yield(b.sessionFactory.New(sess)) {
				break
			}
		}
	}
}

func (b *EventBus) PublishFileChanged(ctx context.Context, event *shared.FileChangedEvent) {
	b.fileChangedTopic.Publish(ctx, event)
}

func (b *EventBus) SubscribeFileChanged(ctx context.Context) iter.Seq2[*shared.FileChangedEvent, error] {
	return b.fileChangedTopic.Subscribe(ctx)
}

// 确保实现接口
var _ dsession.EventBus = (*EventBus)(nil)
