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

// SendNotificationResult 发送通知的结果
type SendNotificationResult struct {
	Notification *Notification
	DidCreate    bool
}

// SendNotification 发送或覆盖通知
func (s *Service) SendNotification(
	ctx context.Context,
	tag string,
	channel string,
	title string,
	body string,
	priority shared.NotificationPriority,
	opts ...shared.SendNotificationOption,
) (*SendNotificationResult, error) {
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
		existing.Update(
			shared.WithUpdateTitle(title),
			shared.WithUpdateBody(body),
			shared.WithUpdatePriority(priority),
			shared.WithUpdateNotAfter(options.NotAfter),
			shared.WithUpdateNotBefore(options.NotBefore),
			shared.WithUpdateDetailsURL(options.DetailsURL),
			shared.WithUpdateTime(now),
		)
		notif = existing
	} else {
		notif, err = s.factory.New(tag, channel, title, body, priority, opts...)
		if err != nil {
			return nil, err
		}
	}

	actualDidCreate, err := s.repo.Save(ctx, notif)
	if err != nil {
		return nil, err
	}

	return &SendNotificationResult{
		Notification: notif,
		DidCreate:    actualDidCreate,
	}, nil
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

// UnsendNotification 撤回（删除）通知，通过 tag 查找
func (s *Service) UnsendNotification(ctx context.Context, tag string) error {
	notif, err := s.repo.GetByTag(ctx, tag)
	if err != nil {
		return err
	}

	return s.repo.Delete(ctx, notif.ID().String())
}

// #endregion

// #region Channels

// GetChannels 获取所有频道及其统计信息
func (s *Service) GetChannels(ctx context.Context, filters shared.NotificationFilters) ([]*ChannelStats, error) {
	var results []*ChannelStats

	for cs, err := range s.repo.Channels(ctx) {
		if err != nil {
			return nil, err
		}
		results = append(results, cs)
	}

	return results, nil
}

// #endregion