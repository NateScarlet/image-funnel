package directory

import (
	appsession "main/internal/application/session"
	"main/internal/domain/directory"
)

// Handler 目录应用层处理器
type Handler struct {
	dirAnalyzer directory.Analyzer
	eventBus    appsession.EventBus
	dtoFactory  *DTOFactory

	filterBuilder *directory.FilterBuilder
	repo          directory.Repository
	dirSvc        *directory.Service
}

// NewHandler 创建目录处理器
func NewHandler(
	dirAnalyzer directory.Analyzer,
	eventBus appsession.EventBus,
	dtoFactory *DTOFactory,
	filterBuilder *directory.FilterBuilder,
	repo directory.Repository,
	dirSvc *directory.Service,
) *Handler {
	return &Handler{
		dirAnalyzer:   dirAnalyzer,
		eventBus:      eventBus,
		dtoFactory:    dtoFactory,
		filterBuilder: filterBuilder,
		repo:          repo,
		dirSvc:        dirSvc,
	}
}