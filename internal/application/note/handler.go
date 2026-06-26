package note

import (
	"main/internal/domain/directory"
	"main/internal/domain/note"
	"main/internal/pubsub"
	"main/internal/shared"
)

type Handler struct {
	repo             note.Repository
	service          *note.Service
	dirSvc           *directory.Service
	fileChangedSub   pubsub.Topic[*shared.FileChangedEvent]
	dtoFactory       *DTOFactory
	filterBuilder    *note.FilterBuilder
}

func NewHandler(
	repo note.Repository,
	service *note.Service,
	dirSvc *directory.Service,
	fileChangedSub pubsub.Topic[*shared.FileChangedEvent],
	dtoFactory *DTOFactory,
	filterBuilder *note.FilterBuilder,
) *Handler {
	return &Handler{
		repo:           repo,
		service:        service,
		dirSvc:         dirSvc,
		fileChangedSub: fileChangedSub,
		dtoFactory:     dtoFactory,
		filterBuilder:  filterBuilder,
	}
}