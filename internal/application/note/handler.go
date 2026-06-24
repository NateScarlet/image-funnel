package note

import (
	"context"
	"iter"

	"main/internal/domain/directory"
	"main/internal/domain/note"
	"main/internal/shared"
)

type EventBus interface {
	SubscribeFileChanged(ctx context.Context) iter.Seq2[*shared.FileChangedEvent, error]
}

type Handler struct {
	repo          note.Repository
	service       *note.Service
	dirSvc        *directory.Service
	ebus          EventBus
	dtoFactory    *DTOFactory
	filterBuilder *note.FilterBuilder
}

func NewHandler(repo note.Repository, service *note.Service, dirSvc *directory.Service, ebus EventBus, dtoFactory *DTOFactory, filterBuilder *note.FilterBuilder) *Handler {
	return &Handler{
		repo:          repo,
		service:       service,
		dirSvc:        dirSvc,
		ebus:          ebus,
		dtoFactory:    dtoFactory,
		filterBuilder: filterBuilder,
	}
}