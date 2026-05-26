package directory

import (
	"context"
	"iter"
	appsession "main/internal/application/session"
	"main/internal/domain/directory"
	"main/internal/scalar"
	"main/internal/shared"
	"path/filepath"
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

// Directory 查询目录信息
func (h *Handler) Directory(ctx context.Context, id scalar.ID) (*shared.DirectoryDTO, error) {
	dirInfo, err := h.dirSvc.GetDirectory(ctx, id)
	if err != nil {
		return nil, err
	}

	relPath := dirInfo.RelPath()
	var parentID scalar.ID
	if relPath != "." {
		parentPath := filepath.Dir(relPath)
		if parentPath != "." {
			parentDir, err := h.repo.Get(ctx, parentPath)
			if err != nil {
				return nil, err
			}
			parentID = parentDir.ID()
		} else {
			rootDir, err := h.repo.Get(ctx, ".")
			if err != nil {
				return nil, err
			}
			parentID = rootDir.ID()
		}
	}

	return h.dtoFactory.New(dirInfo, parentID, relPath == "."), nil
}

// RootDirectory 查询根目录信息
func (h *Handler) RootDirectory(ctx context.Context) (*shared.DirectoryDTO, error) {
	dir, err := h.repo.Get(ctx, ".")
	if err != nil {
		return nil, err
	}
	return h.Directory(ctx, dir.ID())
}

// DirectoryStats 查询目录统计信息
func (h *Handler) DirectoryStats(ctx context.Context, id scalar.ID) (*shared.DirectoryStatsDTO, error) {
	dir, err := h.dirSvc.GetDirectory(ctx, id)
	if err != nil {
		return nil, err
	}
	stats, err := h.dirAnalyzer.Analyze(ctx, dir.RelPath())
	if err != nil {
		return nil, err
	}

	return h.dtoFactory.NewDirectoryStatsDTO(stats)
}

// Directories 查询子目录列表
func (h *Handler) Directories(ctx context.Context, parentID scalar.ID) ([]*shared.DirectoryDTO, error) {
	parentDir, err := h.dirSvc.GetDirectory(ctx, parentID)
	if err != nil {
		return nil, err
	}

	var result []*shared.DirectoryDTO
	for dir, err := range h.repo.Find(ctx, parentDir.RelPath()) {
		if err != nil {
			return nil, err
		}
		dirDTO := h.dtoFactory.New(dir, parentID, false)
		result = append(result, dirDTO)
	}

	return result, nil
}

// DirectoryChanged 订阅目录变更事件
// 根据过滤器返回变更的目录信息
func (h *Handler) DirectoryChanged(ctx context.Context, filters shared.DirectoryFilters) iter.Seq2[*shared.DirectoryDTO, error] {
	return func(yield func(*shared.DirectoryDTO, error) bool) {
		var filter = h.filterBuilder.Build(filters)
		for event, err := range h.eventBus.SubscribeFileChanged(ctx) {
			if !func() bool {
				if err != nil {
					return yield(nil, err)
				}
				dir, err := h.dirSvc.GetDirectory(ctx, event.DirectoryID)
				if err != nil {
					return yield(nil, err)
				}
				if filter(dir) {
					return yield(h.dtoFactory.New(dir, event.DirectoryID, false), nil)
				}
				return true
			}() {
				return
			}

		}
	}
}
