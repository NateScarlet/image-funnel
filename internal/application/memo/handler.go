package memo

import (
	"context"
	"iter"

	"main/internal/domain/directory"
	"main/internal/domain/memo"
	"main/internal/shared"
)

type EventBus interface {
	SubscribeFileChanged(ctx context.Context) iter.Seq2[*shared.FileChangedEvent, error]
}

type Handler struct {
	repo          memo.Repository
	service       *memo.Service
	dirSvc        *directory.Service
	ebus          EventBus
	dtoFactory    *DTOFactory
	filterBuilder *memo.FilterBuilder
}

func NewHandler(repo memo.Repository, service *memo.Service, dirSvc *directory.Service, ebus EventBus, dtoFactory *DTOFactory, filterBuilder *memo.FilterBuilder) *Handler {
	return &Handler{
		repo:          repo,
		service:       service,
		dirSvc:        dirSvc,
		ebus:          ebus,
		dtoFactory:    dtoFactory,
		filterBuilder: filterBuilder,
	}
}