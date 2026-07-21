package notification

import (
	"context"
	"time"

	"main/internal/apperror"
	"main/internal/pubsub"
	"main/internal/scalar"
	"main/internal/shared"
)

// Service 协调通知领域的业务规则
type Service struct {
	repo    Repository
	factory *Factory
	topic   pubsub.Topic[*shared.NotificationChangedEventDTO]
}

// NewService 实例化领域服务
func NewService(repo Repository, factory *Factory, topic pubsub.Topic[*shared.NotificationChangedEventDTO]) *Service {
	return &Service{repo: repo, factory: factory, topic: topic}
}

// #region Send

// Send 发送或覆盖通知
func (s *Service) Send(
	ctx context.Context,
	tag string,
	channel string,
	title string,
	opts ...shared.SendNotificationOption,
) (*shared.SendNotificationResult, error) {
	existing, err := apperror.IgnoreNotFound(s.repo.GetByTag(ctx, tag))
	if err != nil {
		return nil, err
	}

	var notif *Notification

	if existing != nil {
		if existing.Channel() != channel {
			return nil, apperror.New(
				"CHANNEL_CONFLICT",
				"notification with tag "+tag+" already exists in channel "+existing.Channel(),
				"标签 "+tag+" 已存在于频道 "+existing.Channel()+" 中",
			)
		}
		now := time.Now()
		options := shared.NewSendNotificationOptions(opts...)
		// 使用独立 setter 而非一个巨大的 Update
		if err := existing.setTitle(title); err != nil {
			return nil, err
		}
		if err := existing.setBody(options.Body()); err != nil {
			return nil, err
		}
		if err := existing.setPriority(options.Priority()); err != nil {
			return nil, err
		}
		if !options.NotAfter().IsZero() {
			if err := existing.setNotAfter(options.NotAfter()); err != nil {
				return nil, err
			}
		}
		if !options.NotBefore().IsZero() {
			if err := existing.setNotBefore(options.NotBefore()); err != nil {
				return nil, err
			}
		}
		if !options.DetailsURL().IsZero() {
			if err := existing.setDetailsURL(options.DetailsURL()); err != nil {
				return nil, err
			}
		}
		if err := existing.setUpdatedAt(now); err != nil {
			return nil, err
		}
		notif = existing
	} else {
		notif, err = s.factory.New(tag, channel, title, opts...)
		if err != nil {
			return nil, err
		}
	}

	didCreate, err := s.repo.Save(ctx, notif)
	if err != nil {
		return nil, err
	}

	// 领域层发布事件，携带 NotificationID 让接口层按需查询
	eventType := shared.NotificationEventTypeSent
	if !didCreate {
		eventType = shared.NotificationEventTypeUpdated
	}
	if err := s.topic.Publish(ctx, &shared.NotificationChangedEventDTO{
		Event:          eventType,
		NotificationID: notif.ID(),
	}); err != nil {
		return nil, err
	}

	return shared.NewSendNotificationResult(notif.ID(), didCreate), nil
}

// #endregion

// #region Update

// Update 更新通知元数据，使用 shared.UpdateNotificationOption 不可变选项
func (s *Service) Update(ctx context.Context, id scalar.ID, opts ...shared.UpdateNotificationOption) error {
	notif, err := s.repo.Get(ctx, id.String())
	if err != nil {
		return err
	}

	options := shared.NewUpdateNotificationOptions(opts...)
	now := time.Now()

	// 基于选项值进行操作，而非由选项直接修改领域对象
	if v := options.ReadAt(); v != nil {
		notif.readAt = *v
		notif.updatedAt = now
	}
	if v := options.DismissedAt(); v != nil {
		notif.dismissedAt = *v
		if !v.IsZero() && notif.readAt.IsZero() {
			notif.readAt = *v
		}
		notif.updatedAt = now
	}

	_, err = s.repo.Save(ctx, notif)
	if err != nil {
		return err
	}

	// 领域层发布事件，携带 NotificationID 让接口层按需查询
	if err := s.topic.Publish(ctx, &shared.NotificationChangedEventDTO{
		Event:          shared.NotificationEventTypeUpdated,
		NotificationID: id,
	}); err != nil {
		return err
	}

	return nil
}

// #endregion

// #region Unsend

// Unsend 撤回通知，通过 tag 查找，标记 notAfter 为当前时间
func (s *Service) Unsend(ctx context.Context, tag string) (scalar.ID, error) {
	notif, err := apperror.IgnoreNotFound(s.repo.GetByTag(ctx, tag))
	if err != nil {
		return scalar.ID{}, err
	}
	if notif == nil {
		// 通知不存在，返回空 ID
		return scalar.ID{}, nil
	}

	now := time.Now()
	if err := notif.setNotAfter(now); err != nil {
		return scalar.ID{}, err
	}

	_, err = s.repo.Save(ctx, notif)
	if err != nil {
		return scalar.ID{}, err
	}

	// 领域层发布事件，携带 NotificationID 让接口层按需查询
	if err := s.topic.Publish(ctx, &shared.NotificationChangedEventDTO{
		Event:          shared.NotificationEventTypeUnsent,
		NotificationID: notif.ID(),
	}); err != nil {
		return scalar.ID{}, err
	}

	return notif.ID(), nil
}

// #endregion