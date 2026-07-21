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
		DetailsURL:  n.DetailsURL(),
	}
}

// NewChannel 转换频道统计数据
func (f *DTOFactory) NewChannel(cs *domnotif.ChannelStats) *shared.NotificationChannelDTO {
	if cs == nil {
		return nil
	}
	return &shared.NotificationChannelDTO{
		Channel:              cs.Channel,
		UnreadCount:          cs.UnreadCount,
		LatestNotificationID: cs.LatestNotificationID,
	}
}
