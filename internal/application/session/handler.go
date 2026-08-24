package session

import (
	"context"

	appimage "main/internal/application/image"
	"main/internal/domain/session"
	"main/internal/pubsub"
	"main/internal/scalar"
	"main/internal/shared"

	"go.uber.org/zap"
)

// LastSessionSaver 定义了保存目录最后一次活跃会话历史的本地接口，不限定具体的存储实现与存储位置
type LastSessionSaver interface {
	SaveLastSession(
		ctx context.Context,
		directoryID scalar.ID,
		sessionID scalar.ID,
		filter *shared.ImageFilters,
		targetKeep int,
	) error
}

type Handler struct {
	sessionService      *session.Service
	dtoFactory          *DTOFactory
	imageDTOFactory     *appimage.DTOFactory
	logger              *zap.Logger
	lastSessionSaver    LastSessionSaver
	sessionSavedSub     pubsub.Topic[scalar.ID]
	sessionCommittedSub pubsub.Topic[*shared.SessionCommittedEvent]
}

func NewHandler(
	sessionService *session.Service,
	dtoFactory *DTOFactory,
	imageDTOFactory *appimage.DTOFactory,
	logger *zap.Logger,
	lastSessionSaver LastSessionSaver,
	sessionSavedSub pubsub.Topic[scalar.ID],
	sessionCommittedSub pubsub.Topic[*shared.SessionCommittedEvent],
) *Handler {
	return &Handler{
		sessionService:      sessionService,
		dtoFactory:          dtoFactory,
		imageDTOFactory:     imageDTOFactory,
		logger:              logger,
		lastSessionSaver:    lastSessionSaver,
		sessionSavedSub:     sessionSavedSub,
		sessionCommittedSub: sessionCommittedSub,
	}
}
