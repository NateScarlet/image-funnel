package notification

import (
	domnotif "main/internal/domain/notification"
	"main/internal/shared"
)

// DTOFactory 负责将领域实体转换成 DTO
type DTOFactory struct{}

func NewDTOFactory() *DTOFactory {
	return &DTOFactory{}
}

// New 转换通知实体
func (f *DTOFactory) New(n *domnotif.Notification) *shared.NotificationDTO {
	if n == nil {
		return nil
	}
	return &shared.NotificationDTO{
		ID:          n.ID(),
		Tag:         n.Tag(),
		Channel:     n.Channel(),
		Title:       n.Title(),
		Body:        n.Body(),
		Priority:    n.Priority(),
		Status:      n.Status(),
		ReadAt:      n.ReadAt(),
		DismissedAt: n.DismissedAt(),
		NotAfter:    n.NotAfter(),
		NotBefore:   n.NotBefore(),
		CreatedAt:   n.CreatedAt(),
		UpdatedAt:   n.UpdatedAt(),
		DetailURL:   n.DetailURL(),
	}
}

// NewChannel 转换频道统计数据
func (f *DTOFactory) NewChannel(cs *domnotif.ChannelStats) *shared.NotificationChannelDTO {
	if cs == nil {
		return nil
	}
	return f.NewChannelWithData(cs.Channel, cs.UnreadCount, cs.LatestNotification)
}

// NewChannelWithData 使用自定义数据构造 DTO，保证 DTO 边界内聚
func (f *DTOFactory) NewChannelWithData(channel string, unreadCount int, latest *domnotif.Notification) *shared.NotificationChannelDTO {
	return &shared.NotificationChannelDTO{
		Channel:            channel,
		UnreadCount:        unreadCount,
		LatestNotification: f.New(latest),
	}
}
