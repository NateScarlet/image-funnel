package directory

import (
	"main/internal/domain/directory"
	"main/internal/pubsub"
	"main/internal/shared"
)

// Handler 目录应用层处理器
type Handler struct {
	dirAnalyzer    directory.Analyzer
	dtoFactory     *DTOFactory
	filterBuilder  *directory.FilterBuilder
	repo           directory.Repository
	dirSvc         *directory.Service
	fileChangedSub pubsub.Topic[*shared.FileChangedEvent]
}

// NewHandler 创建目录处理器
func NewHandler(
	dirAnalyzer directory.Analyzer,
	dtoFactory *DTOFactory,
	filterBuilder *directory.FilterBuilder,
	repo directory.Repository,
	dirSvc *directory.Service,
	fileChangedSub pubsub.Topic[*shared.FileChangedEvent],
) *Handler {
	return &Handler{
		dirAnalyzer:    dirAnalyzer,
		dtoFactory:     dtoFactory,
		filterBuilder:  filterBuilder,
		repo:           repo,
		dirSvc:         dirSvc,
		fileChangedSub: fileChangedSub,
	}
}