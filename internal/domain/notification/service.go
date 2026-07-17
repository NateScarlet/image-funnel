package notification

import (
	"context"
	"fmt"
	"time"

	"main/internal/apperror"
	"main/internal/scalar"
	"main/internal/shared"

	"github.com/google/uuid"
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

// SendNotificationInput 发送通知的输入参数
type SendNotificationInput struct {
	Tag        string
	Channel    string
	Title      string
	Body       string
	Priority   shared.NotificationPriority
	NotAfter   time.Time
	NotBefore  time.Time
	DetailsURL scalar.URI
}

// SendNotificationResult 发送通知的结果
type SendNotificationResult struct {
	Notification *Notification
	DidCreate    bool
}

// SendNotification 发送或覆盖通知
func (s *Service) SendNotification(ctx context.Context, input SendNotificationInput) (*SendNotificationResult, error) {
	// 校验：tag 必须是 UUID
	if _, err := uuid.Parse(input.Tag); err != nil {
		return nil, fmt.Errorf("tag must be a valid UUID: %w", err)
	}
	// 校验：title 必填
	if input.Title == "" {
		return nil, fmt.Errorf("title is required")
	}
	// 优先级默认普通
	if input.Priority.IsZero() {
		input.Priority = shared.NotificationPriorityNormal
	}
	// notBefore 默认现在
	if input.NotBefore.IsZero() {
		input.NotBefore = time.Now()
	}
	// notAfter 默认一周后
	if input.NotAfter.IsZero() {
		input.NotAfter = time.Now().Add(7 * 24 * time.Hour)
	}

	existing, err := s.repo.GetByTag(ctx, input.Tag)
	if err != nil {
		return nil, err
	}

	var notif *Notification

	if existing != nil {
		if existing.Channel() != input.Channel {
			return nil, apperror.New(
				"CHANNEL_CONFLICT",
				"notification with tag "+input.Tag+" already exists in channel "+existing.Channel(),
				"标签 "+input.Tag+" 已存在于频道 "+existing.Channel()+" 中",
			)
		}
		existing.Update(input.Title, input.Body, input.Priority, input.NotAfter, input.NotBefore, input.DetailsURL, time.Now())
		notif = existing
	} else {
		notif = s.factory.New(input.Tag, input.Channel, input.Title, input.Body, input.Priority, input.NotAfter, input.NotBefore, input.DetailsURL)
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