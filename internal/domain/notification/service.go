package notification

import (
	"time"

	"main/internal/shared"
)

// Service 协调通知领域的业务规则
type Service struct {
	repo Repository
}

// NewService 实例化领域服务
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// CreateNew 创建并保存新通知（ID 自动生成）
func (s *Service) CreateNew(
	tag string,
	channel string,
	title string,
	body string,
	priority shared.NotificationPriority,
	notAfter time.Time,
	notBefore time.Time,
) *Notification {
	now := time.Now()
	return newNotification(
		tag, channel, title, body, priority,
		time.Time{}, time.Time{}, notAfter, notBefore, now, now,
	)
}
