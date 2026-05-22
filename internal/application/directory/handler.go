package directory

import (
	"context"
	"iter"
	appimage "main/internal/application/image"
	appsession "main/internal/application/session"
	"main/internal/domain/directory"
	"main/internal/domain/image"
	"main/internal/pagination"
	"main/internal/scalar"
	"main/internal/shared"
	"path/filepath"
)

// Handler 目录应用层处理器
type Handler struct {
	scanner         directory.Scanner
	eventBus        appsession.EventBus
	dtoFactory      *DirectoryDTOFactory
	imageDTOFactory *appimage.ImageDTOFactory

	filterBuilder *directory.FilterBuilder
	repo          directory.Repository
}

// NewHandler 创建目录处理器
func NewHandler(
	scanner directory.Scanner,
	eventBus appsession.EventBus,
	imageDTOFactory *appimage.ImageDTOFactory,
	repo directory.Repository,
) *Handler {
	return &Handler{
		scanner:         scanner,
		eventBus:        eventBus,
		dtoFactory:      NewDirectoryDTOFactory(imageDTOFactory),
		imageDTOFactory: imageDTOFactory,
		filterBuilder:   directory.NewFilterBuilder(),
		repo:            repo,
	}
}

// Directory 查询目录信息
func (h *Handler) Directory(ctx context.Context, id scalar.ID) (*shared.DirectoryDTO, error) {
	dirInfo, err := h.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	relPath := dirInfo.RelPath()
	var parentID scalar.ID
	if relPath != "." {
		parentPath := filepath.Dir(relPath)
		if parentPath != "." {
			parentID = directory.EncodeID(parentPath)
		} else {
			parentID = directory.EncodeID(".")
		}
	}

	return h.dtoFactory.New(dirInfo, parentID, relPath == "."), nil
}

// DirectoryStats 查询目录统计信息
func (h *Handler) DirectoryStats(ctx context.Context, id scalar.ID) (*shared.DirectoryStatsDTO, error) {
	path, err := directory.DecodeID(id)
	if err != nil {
		return nil, err
	}
	stats, err := h.scanner.AnalyzeDirectory(ctx, path)
	if err != nil {
		return nil, err
	}

	return h.dtoFactory.NewDirectoryStatsDTO(stats)
}

// Directories 查询子目录列表
func (h *Handler) Directories(ctx context.Context, parentID scalar.ID) ([]*shared.DirectoryDTO, error) {
	path, err := directory.DecodeID(parentID)
	if err != nil {
		return nil, err
	}

	var result []*shared.DirectoryDTO
	for dir, err := range h.scanner.ScanDirectories(ctx, path) {
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
		// 订阅文件变更事件
		for event, err := range h.eventBus.SubscribeFileChanged(ctx) {
			if !func() bool {
				if err != nil {
					return yield(nil, err)
				}
				dir, err := h.repo.Get(ctx, event.DirectoryID)
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

// ImageSaved 订阅目录内图片新增/更新事件（CREATE/WRITE）
// 文件创建或写入时，扫描该路径并返回完整 ImageDTO
func (h *Handler) ImageSaved(ctx context.Context, filter *shared.ImageFilters) iter.Seq2[*shared.ImageDTO, error] {
	return func(yield func(*shared.ImageDTO, error) bool) {
		imageFilter := image.BuildImageFilter(filter)
		for event, err := range h.eventBus.SubscribeFileChanged(ctx) {
			if !func() bool {
				if err != nil {
					return yield(nil, err)
				}
				// 只处理新增和写入动作
				if event.Action != shared.FileActionCreate && event.Action != shared.FileActionWrite {
					return true
				}
				img, err := h.scanner.LookupImage(ctx, event.RelPath)
				if err != nil {
					// 文件可能已被快速删除，跳过而不向客户端报错
					return true
				}
				if !imageFilter(img) {
					return true
				}
				dto, err := h.imageDTOFactory.New(img)
				if err != nil {
					return yield(nil, err)
				}
				return yield(dto, nil)
			}() {
				return
			}
		}
	}
}

// ImageDeleted 订阅目录内图片删除/移走事件（REMOVE/RENAME）
// 文件消失时，返回原来的图片 ID 以便前端从列表中移除
func (h *Handler) ImageDeleted(ctx context.Context, filter *shared.ImageFilters) iter.Seq2[scalar.ID, error] {
	return func(yield func(scalar.ID, error) bool) {
		// 对于删除事件，我们只应用目录ID和ID筛选，因为图片已经被删除
		// 所以我们不能构建完整的 Image 对象来应用其他筛选
		var allowedDirectoryIDs map[scalar.ID]bool
		var allowedIDs map[scalar.ID]bool
		if filter != nil {
			if len(filter.DirectoryID) > 0 {
				allowedDirectoryIDs = make(map[scalar.ID]bool)
				for _, id := range filter.DirectoryID {
					allowedDirectoryIDs[id] = true
				}
			}
			if len(filter.ID) > 0 {
				allowedIDs = make(map[scalar.ID]bool)
				for _, id := range filter.ID {
					allowedIDs[id] = true
				}
			}
		}

		for event, err := range h.eventBus.SubscribeFileChanged(ctx) {
			if !func() bool {
				if err != nil {
					return yield(scalar.ToID(""), err)
				}
				// 只处理删除和重命名动作
				if event.Action != shared.FileActionRemove && event.Action != shared.FileActionRename {
					return true
				}
				// 应用目录ID筛选
				if allowedDirectoryIDs != nil && !allowedDirectoryIDs[event.DirectoryID] {
					return true
				}
				// 尝试查找图片以获取其完整ID（虽然文件已删除，但可能还在缓存中）
				img, err := h.scanner.LookupImage(ctx, event.RelPath)
				if err != nil || img == nil {
					// 文件已删除，无法查找，尝试从路径重建ID
					// 但我们没有modtime，所以无法准确重建
					return true
				}
				// 应用ID筛选
				if allowedIDs != nil && !allowedIDs[img.ID()] {
					return true
				}
				return yield(img.ID(), nil)
			}() {
				return
			}
		}
	}
}

// #region 目录内图片过滤与分页查询

// Images 获取目录下的图片列表，支持过滤与基于 Relay 规范的游标分页
func (h *Handler) Images(
	ctx context.Context,
	id scalar.ID,
	filterBy shared.ImageFilters,
	first *int,
	after *string,
) (connection *shared.ImageConnectionDTO, err error) {
	// 如果未指定分页大小，提供合理的缺省限制值以保护后端性能
	if first == nil {
		defaultFirst := 100
		first = &defaultFirst
	}

	// 还原目录真实相对路径
	relPath, err := directory.DecodeID(id)
	if err != nil {
		return nil, err
	}

	// 构造星级、颜色标签、文本关键字等多维度图片过滤器
	imgFilter := image.BuildImageFilter(&filterBy)

	// 创建游标连接缓存，用于处理分页边界与数据打包
	builder := pagination.NewConnectionBufferBuilder[*shared.ImageDTO, *shared.ImageEdgeDTO, *shared.ImageConnectionDTO]()
	buf := builder(
		func(item *shared.ImageDTO, cursor string) (*shared.ImageEdgeDTO, error) {
			return &shared.ImageEdgeDTO{
				Node:   item,
				Cursor: cursor,
			}, nil
		},
		func(edges []*shared.ImageEdgeDTO, pageInfo pagination.PageInfo) (*shared.ImageConnectionDTO, error) {
			var nodes = make([]*shared.ImageDTO, len(edges))
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
			return &shared.ImageConnectionDTO{
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

	// 从 GraphQL 输入转换为通用的分页选项
	options := pagination.OptionFromInput(after, nil, first, nil)

	// 定义流式扫描与过滤序列
	filteredSeq := func(yield func(*shared.ImageDTO, error) bool) {
		for img, scanErr := range h.scanner.Scan(ctx, relPath) {
			if scanErr != nil {
				if !yield(nil, scanErr) {
					return
				}
				continue
			}
			if !imgFilter(img) {
				continue
			}
			dto, factoryErr := h.imageDTOFactory.New(img)
			if factoryErr != nil {
				if !yield(nil, factoryErr) {
					return
				}
				continue
			}
			if !yield(dto, nil) {
				return
			}
		}
	}

	// 利用 pagination 工具包完成游标分页截取与 PageInfo 计算
	err = pagination.ByIndexE(filteredSeq, buf, options...)
	if err != nil {
		return nil, err
	}

	return buf.Value()
}

// Memos 获取目录下的备忘录列表，支持过滤与基于 Relay 规范的游标分页
func (h *Handler) Memos(
	ctx context.Context,
	id scalar.ID,
	filterBy shared.MemoFilters,
	first *int,
	after *string,
) (connection *shared.MemoConnectionDTO, err error) {
	if first == nil {
		defaultFirst := 100
		first = &defaultFirst
	}

	relPath, err := directory.DecodeID(id)
	if err != nil {
		return nil, err
	}

	// 构造游标连接缓存
	builder := pagination.NewConnectionBufferBuilder[*shared.MemoDTO, *shared.MemoEdgeDTO, *shared.MemoConnectionDTO]()
	buf := builder(
		func(item *shared.MemoDTO, cursor string) (*shared.MemoEdgeDTO, error) {
			return &shared.MemoEdgeDTO{
				Node:   item,
				Cursor: cursor,
			}, nil
		},
		func(edges []*shared.MemoEdgeDTO, pageInfo pagination.PageInfo) (*shared.MemoConnectionDTO, error) {
			var nodes = make([]*shared.MemoDTO, len(edges))
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
			return &shared.MemoConnectionDTO{
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

	filteredSeq := func(yield func(*shared.MemoDTO, error) bool) {
		for m, scanErr := range h.scanner.ScanMemos(ctx, relPath) {
			if scanErr != nil {
				if !yield(nil, scanErr) {
					return
				}
				continue
			}
			dto := &shared.MemoDTO{
				ID:         m.ID(),
				AbsPath:    m.AbsPath(),
				Content:    m.Content(),
				RawContent: m.RawContent(),
				Hidden:     m.Hidden(),
			}

			// 应用过滤条件
			if len(filterBy.ID) > 0 {
				found := false
				for _, filterId := range filterBy.ID {
					if filterId == dto.ID {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}
			if filterBy.Hidden != nil {
				if *filterBy.Hidden != dto.Hidden {
					continue
				}
			}

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

// #endregion
