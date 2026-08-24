package note

import (
	"main/internal/domain/directory"
	"main/internal/domain/note"
	"main/internal/pubsub"
	"main/internal/shared"

	"go.uber.org/zap"
)

type Handler struct {
	repo           note.Repository
	service        *note.Service
	dirSvc         *directory.Service
	fileChangedSub pubsub.Topic[*shared.FileChangedEvent]
	dtoFactory     *DTOFactory
	filterBuilder  *note.FilterBuilder
	logger         *zap.Logger
}

func NewHandler(
	logger *zap.Logger,
	repo note.Repository,
	service *note.Service,
	dirSvc *directory.Service,
	fileChangedSub pubsub.Topic[*shared.FileChangedEvent],
	dtoFactory *DTOFactory,
	filterBuilder *note.FilterBuilder,
) *Handler {
	return &Handler{
		logger:         logger,
		repo:           repo,
		service:        service,
		dirSvc:         dirSvc,
		fileChangedSub: fileChangedSub,
		dtoFactory:     dtoFactory,
		filterBuilder:  filterBuilder,
	}
}
