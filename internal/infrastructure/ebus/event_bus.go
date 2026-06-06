package ebus

import (
	"context"
	"iter"

	appdevice "main/internal/application/device"
	appsession "main/internal/application/session"
	ddevice "main/internal/domain/device"
	"main/internal/domain/pairing"
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

	// auth topics
	prCreatedTopic pubsub.Topic[*shared.PairingRequestDTO]
	prUpdatedTopic pubsub.Topic[*shared.PairingRequestDTO]

	// device topics
	deviceSavedTopic   pubsub.Topic[*shared.DeviceDTO]
	deviceDeletedTopic pubsub.Topic[scalar.ID]

	sessionRepo    dsession.Repository
	sessionFactory *appsession.DTOFactory
	deviceFactory  *appdevice.DTOFactory
}

func NewEventBus(
	sessionTopic pubsub.Topic[scalar.ID],
	fileChangedTopic pubsub.Topic[*shared.FileChangedEvent],
	prCreatedTopic pubsub.Topic[*shared.PairingRequestDTO],
	prUpdatedTopic pubsub.Topic[*shared.PairingRequestDTO],
	deviceSavedTopic pubsub.Topic[*shared.DeviceDTO],
	deviceDeletedTopic pubsub.Topic[scalar.ID],
	sessionRepo dsession.Repository,
	sessionFactory *appsession.DTOFactory,
	deviceFactory *appdevice.DTOFactory,
) *EventBus {
	return &EventBus{
		sessionTopic:       sessionTopic,
		fileChangedTopic:   fileChangedTopic,
		prCreatedTopic:     prCreatedTopic,
		prUpdatedTopic:     prUpdatedTopic,
		deviceSavedTopic:   deviceSavedTopic,
		deviceDeletedTopic: deviceDeletedTopic,
		sessionRepo:        sessionRepo,
		sessionFactory:     sessionFactory,
		deviceFactory:      deviceFactory,
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

func (b *EventBus) PublishPairingRequestCreated(ctx context.Context, dto *shared.PairingRequestDTO) {
	b.prCreatedTopic.Publish(ctx, dto)
}

func (b *EventBus) SubscribePairingRequestCreated(ctx context.Context) iter.Seq2[*shared.PairingRequestDTO, error] {
	return b.prCreatedTopic.Subscribe(ctx)
}

func (b *EventBus) PublishPairingRequestUpdated(ctx context.Context, dto *shared.PairingRequestDTO) {
	b.prUpdatedTopic.Publish(ctx, dto)
}

func (b *EventBus) SubscribePairingRequestUpdated(ctx context.Context) iter.Seq2[*shared.PairingRequestDTO, error] {
	return b.prUpdatedTopic.Subscribe(ctx)
}

func (b *EventBus) PublishDeviceSaved(ctx context.Context, device *ddevice.Device) {
	dto := b.deviceFactory.New(device)
	b.deviceSavedTopic.Publish(ctx, dto)
}

func (b *EventBus) SubscribeDeviceSaved(ctx context.Context) iter.Seq2[*shared.DeviceDTO, error] {
	return b.deviceSavedTopic.Subscribe(ctx)
}

func (b *EventBus) PublishDeviceDeleted(ctx context.Context, id scalar.ID) {
	b.deviceDeletedTopic.Publish(ctx, id)
}

func (b *EventBus) SubscribeDeviceDeleted(ctx context.Context) iter.Seq2[scalar.ID, error] {
	return b.deviceDeletedTopic.Subscribe(ctx)
}

// 确保实现接口
var _ dsession.EventBus = (*EventBus)(nil)
var _ pairing.EventBus = (*EventBus)(nil)
var _ ddevice.EventBus = (*EventBus)(nil)
var _ appdevice.EventBus = (*EventBus)(nil)
