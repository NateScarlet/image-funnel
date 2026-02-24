package ebus

import (
	"context"
	"iter"

	appsession "main/internal/application/session"
	dsession "main/internal/domain/session"
	"main/internal/pubsub"
	"main/internal/scalar"
	"main/internal/shared"
)

// EventBus 事件总线实现
type EventBus struct {
	// sessionTopic 只传递 ID，订阅者在接收后自行 Acquire 获取最新状态，避免跨 goroutine 持有 *Session 指针
	sessionTopic     pubsub.Topic[scalar.ID]
	fileChangedTopic pubsub.Topic[*shared.FileChangedEvent]
	sessionRepo      dsession.Repository
	sessionFactory   *appsession.SessionDTOFactory
}

func NewEventBus(
	sessionTopic pubsub.Topic[scalar.ID],
	fileChangedTopic pubsub.Topic[*shared.FileChangedEvent],
	sessionRepo dsession.Repository,
	sessionFactory *appsession.SessionDTOFactory,
) *EventBus {
	return &EventBus{
		sessionTopic:     sessionTopic,
		fileChangedTopic: fileChangedTopic,
		sessionRepo:      sessionRepo,
		sessionFactory:   sessionFactory,
	}
}

func (b *EventBus) SubscribeSession(ctx context.Context) iter.Seq2[*shared.SessionDTO, error] {
	return func(yield func(*shared.SessionDTO, error) bool) {
		for id, err := range b.sessionTopic.Subscribe(ctx) {
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			// 重新 Acquire 以持锁读取，避免并发 map 访问竞态
			sess, release, err := b.sessionRepo.Acquire(ctx, id)
			if err != nil {
				// session 可能已被清理，忽略
				continue
			}
			dto, err := b.sessionFactory.New(sess)
			release()

			if !yield(dto, err) {
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
