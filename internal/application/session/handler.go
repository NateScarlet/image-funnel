package session

import (
	"context"

	appimage "main/internal/application/image"
	"main/internal/domain/session"
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
	SaveLastSessionCommitActions(
		ctx context.Context,
		directoryID scalar.ID,
		sessionID scalar.ID,
		commitActions *shared.WriteActions,
	) error
}

type Handler struct {
	sessionService  *session.Service
	eventBus        EventBus
	dtoFactory      *DTOFactory
	imageDTOFactory *appimage.DTOFactory
	logger          *zap.Logger
	lastSessionSaver LastSessionSaver
}

func NewHandler(
	sessionService *session.Service,
	eventBus EventBus,
	dtoFactory *DTOFactory,
	imageDTOFactory *appimage.DTOFactory,
	logger *zap.Logger,
	lastSessionSaver LastSessionSaver,
) *Handler {
	return &Handler{
		sessionService:  sessionService,
		eventBus:        eventBus,
		dtoFactory:      dtoFactory,
		imageDTOFactory: imageDTOFactory,
		logger:          logger,
		lastSessionSaver: lastSessionSaver,
	}
}