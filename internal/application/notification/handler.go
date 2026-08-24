package notification

import (
	domnotif "main/internal/domain/notification"
	"main/internal/pubsub"
	"main/internal/shared"

	"go.uber.org/zap"
)

// Handler 协调通知系统的应用层逻辑
type Handler struct {
	repo          domnotif.Repository
	service       *domnotif.Service
	dtoFactory    *DTOFactory
	filterBuilder *domnotif.FilterBuilder
	topic         pubsub.Topic[*shared.NotificationChangedEventDTO]
	logger        *zap.Logger
}

func NewHandler(
	repo domnotif.Repository,
	service *domnotif.Service,
	dtoFactory *DTOFactory,
	filterBuilder *domnotif.FilterBuilder,
	topic pubsub.Topic[*shared.NotificationChangedEventDTO],
	logger *zap.Logger,
) *Handler {
	return &Handler{
		repo:          repo,
		service:       service,
		dtoFactory:    dtoFactory,
		filterBuilder: filterBuilder,
		topic:         topic,
		logger:        logger,
	}
}
