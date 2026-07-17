package notification

import (
	"context"
	"time"

	"main/internal/apperror"
	"main/internal/scalar"
	"main/internal/shared"
)

// Service 协调通知领域的业务规则
type Service struct {
	repo    Repository
	factory *Factory
}

// NewService 实例化领域服务
func NewService(repo Repository) *Service {
	return &Service{repo: repo, factory: &Factory{}}
}

// #region SendNotification

// SendNotification 发送或覆盖通知
func (s *Service) SendNotification(
	ctx context.Context,
	tag string,
	channel string,
	title string,
	opts ...shared.SendNotificationOption,
) (*shared.SendNotificationResult, error) {
	existing, err := s.repo.GetByTag(ctx, tag)
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
		existing.setBody(options.Body())
		existing.setPriority(options.Priority())
		if !options.NotAfter().IsZero() {
			existing.setNotAfter(options.NotAfter())
		}
		if !options.NotBefore().IsZero() {
			existing.setNotBefore(options.NotBefore())
		}
		if !options.DetailsURL().IsZero() {
			existing.setDetailsURL(options.DetailsURL())
		}
		existing.setUpdatedAt(now)
		notif = existing
	} else {
		notif, err = s.factory.New(tag, channel, title, opts...)
		if err != nil {
			return nil, err
		}
	}

	actualDidCreate, err := s.repo.Save(ctx, notif)
	if err != nil {
		return nil, err
	}

	return shared.NewSendNotificationResult(notif.ID(), actualDidCreate), nil
}

// #endregion

// #region UpdateNotification

// UpdateNotification 更新通知元数据（已读时间、关闭时间）
func (s *Service) UpdateNotification(ctx context.Context, id scalar.ID, readAt *time.Time, dismissedAt *time.Time) (*Notification, error) {
	notif, err := s.repo.Get(ctx, id.String())
	if err != nil {
		return nil, err
	}

	now := time.Now()
	if readAt != nil {
		notif.MarkRead(*readAt, now)
	}
	if dismissedAt != nil {
		notif.Dismiss(*dismissedAt, now)
	}

	_, err = s.repo.Save(ctx, notif)
	if err != nil {
		return nil, err
	}

	return notif, nil
}

// #endregion

// #region UnsendNotification

// UnsendNotification 撤回通知，通过 tag 查找，标记 notAfter 为当前时间
func (s *Service) UnsendNotification(ctx context.Context, tag string) (*Notification, error) {
	notif, err := s.repo.GetByTag(ctx, tag)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	notif.setNotAfter(now)

	_, err = s.repo.Save(ctx, notif)
	if err != nil {
		return nil, err
	}

	return notif, nil
}

// #endregion

