package directory

import (
	"main/internal/domain/directory"
	"main/internal/pubsub"
	"time"

	"go.uber.org/zap"
)

// Handler 目录应用层处理器
type Handler struct {
	dirAnalyzer                directory.Analyzer
	dtoFactory                 *DTOFactory
	filterBuilder              *directory.FilterBuilder
	repo                       directory.Repository
	dirSvc                     *directory.Service
	fileChangedSub             pubsub.Topic[*directory.FileChangedEvent]
	dirEntryDeletedBatchWindow time.Duration
	logger                     *zap.Logger
}

// NewHandler 创建目录处理器
func NewHandler(
	logger *zap.Logger,
	dirAnalyzer directory.Analyzer,
	dtoFactory *DTOFactory,
	filterBuilder *directory.FilterBuilder,
	repo directory.Repository,
	dirSvc *directory.Service,
	fileChangedSub pubsub.Topic[*directory.FileChangedEvent],
) *Handler {
	return &Handler{
		logger:         logger,
		dirAnalyzer:    dirAnalyzer,
		dtoFactory:     dtoFactory,
		filterBuilder:  filterBuilder,
		repo:           repo,
		dirSvc:         dirSvc,
		fileChangedSub: fileChangedSub,
	}
}
