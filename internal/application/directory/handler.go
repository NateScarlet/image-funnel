package directory

import (
	"context"
	"iter"
	appsession "main/internal/application/session"
	"main/internal/domain/directory"
	"main/internal/pagination"
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

// Directories 获取子目录列表，支持过滤与基于 Relay 规范的游标分页
func (h *Handler) Directories(
	ctx context.Context,
	parentID scalar.ID,
	filterBy shared.DirectoryFilters,
	first *int,
	after *string,
) (connection *shared.DirectoryConnectionDTO, err error) {
	if first == nil {
		defaultFirst := 100
		first = &defaultFirst
	}

	parentDir, err := h.dirSvc.GetDirectory(ctx, parentID)
	if err != nil {
		return nil, err
	}

	builder := pagination.NewConnectionBufferBuilder[*shared.DirectoryDTO, *shared.DirectoryEdgeDTO, *shared.DirectoryConnectionDTO]()
	buf := builder(
		func(item *shared.DirectoryDTO, cursor string) (*shared.DirectoryEdgeDTO, error) {
			return &shared.DirectoryEdgeDTO{
				Node:   item,
				Cursor: cursor,
			}, nil
		},
		func(edges []*shared.DirectoryEdgeDTO, pageInfo pagination.PageInfo) (*shared.DirectoryConnectionDTO, error) {
			var nodes = make([]*shared.DirectoryDTO, len(edges))
			for i, edge := range edges {
				nodes[i] = edge.Node
			}
			var startCursor, endCursor string
			if pageInfo.StartCursor != nil {
				startCursor = *pageInfo.StartCursor
			}
			if pageInfo.EndCursor != nil {
				endCursor = *pageInfo.EndCursor
			}
			return &shared.DirectoryConnectionDTO{
				Edges: edges,
				Nodes: nodes,
				PageInfo: &shared.PageInfoDTO{
					HasNextPage:     pageInfo.HasNextPage,
					HasPreviousPage: pageInfo.HasPreviousPage,
					StartCursor:     startCursor,
					EndCursor:       endCursor,
				},
			}, nil
		},
	)

	options := pagination.OptionFromInput(after, nil, first, nil)

	filteredSeq := func(yield func(*shared.DirectoryDTO, error) bool) {
		dirFilter := h.filterBuilder.Build(filterBy)
		for dir, scanErr := range h.repo.Find(ctx, parentDir.RelPath()) {
			if scanErr != nil {
				if !yield(nil, scanErr) {
					return
				}
				continue
			}
			if !dirFilter(dir) {
				continue
			}
			dto := h.dtoFactory.New(dir, parentID, false)
			if !yield(dto, nil) {
				return
			}
		}
	}

	err = pagination.ByIndexE(filteredSeq, buf, options...)
	if err != nil {
		return nil, err
	}

	return buf.Value()
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

// DirEntryDeleted 订阅目录内的文件/目录被删除或移走（重命名为其他名字）的事件
func (h *Handler) DirEntryDeleted(ctx context.Context, directoryID *scalar.ID) iter.Seq2[*shared.DirEntryDeletedDTO, error] {
	return func(yield func(*shared.DirEntryDeletedDTO, error) bool) {
		for event, err := range h.eventBus.SubscribeFileChanged(ctx) {
			if !func() bool {
				if err != nil {
					return yield(nil, err)
				}
				if event.Action != shared.FileActionRemove && event.Action != shared.FileActionRename {
					return true
				}
				if directoryID != nil && event.DirectoryID != *directoryID {
					return true
				}

				return yield(&shared.DirEntryDeletedDTO{
					RelPath: event.RelPath,
				}, nil)
			}() {
				return
			}
		}
	}
}
