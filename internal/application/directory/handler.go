package directory

import (
	"context"
	"iter"
	appimage "main/internal/application/image"
	appmemo "main/internal/application/memo"
	appsession "main/internal/application/session"
	"main/internal/domain/directory"
	"main/internal/domain/image"
	"main/internal/domain/memo"
	"main/internal/pagination"
	"main/internal/scalar"
	"main/internal/shared"
	"main/internal/util"
	"path/filepath"
	"slices"
	"time"

	"go.uber.org/zap"
)

// Handler 目录应用层处理器
type Handler struct {
	dirScanner         directory.Scanner
	dirAnalyzer        directory.Analyzer
	imgScanner         image.Scanner
	imgMover           image.Mover
	memoScanner        memo.Scanner
	eventBus           appsession.EventBus
	dtoFactory         *DTOFactory
	imageDTOFactory    *appimage.DTOFactory
	memoDTOFactory     *appmemo.DTOFactory

	filterBuilder     *directory.FilterBuilder
	imageFilterBuilder *image.FilterBuilder
	memoFilterBuilder  *memo.FilterBuilder
	repo              directory.Repository
	logger            *zap.Logger
}

// NewHandler 创建目录处理器
func NewHandler(
	dirScanner directory.Scanner,
	dirAnalyzer directory.Analyzer,
	imgScanner image.Scanner,
	imgMover image.Mover,
	memoScanner memo.Scanner,
	eventBus appsession.EventBus,
	imageDTOFactory *appimage.DTOFactory,
	memoDTOFactory *appmemo.DTOFactory,
	dtoFactory *DTOFactory,
	filterBuilder *directory.FilterBuilder,
	imageFilterBuilder *image.FilterBuilder,
	memoFilterBuilder *memo.FilterBuilder,
	repo directory.Repository,
	logger *zap.Logger,
) *Handler {
	return &Handler{
		dirScanner:         dirScanner,
		dirAnalyzer:        dirAnalyzer,
		imgScanner:         imgScanner,
		imgMover:           imgMover,
		memoScanner:        memoScanner,
		eventBus:           eventBus,
		dtoFactory:         dtoFactory,
		imageDTOFactory:    imageDTOFactory,
		filterBuilder:      filterBuilder,
		imageFilterBuilder: imageFilterBuilder,
		memoFilterBuilder:  memoFilterBuilder,
		repo:               repo,
		logger:             logger,
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
			parentDir, err := h.repo.GetByRelPath(ctx, parentPath)
			if err != nil {
				return nil, err
			}
			parentID = parentDir.ID()
		} else {
			rootDir, err := h.repo.GetByRelPath(ctx, ".")
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
	dir, err := h.repo.GetByRelPath(ctx, ".")
	if err != nil {
		return nil, err
	}
	return h.Directory(ctx, dir.ID())
}

// DirectoryStats 查询目录统计信息
func (h *Handler) DirectoryStats(ctx context.Context, id scalar.ID) (*shared.DirectoryStatsDTO, error) {
	path, err := directory.DecodeID(id)
	if err != nil {
		return nil, err
	}
	stats, err := h.dirAnalyzer.Analyze(ctx, path)
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
	for dir, err := range h.dirScanner.Scan(ctx, path) {
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
		imageFilter := h.imageFilterBuilder.Build(util.UnwrapPointer(filter))
		for event, err := range h.eventBus.SubscribeFileChanged(ctx) {
			if !func() bool {
				if err != nil {
					return yield(nil, err)
				}
				// 只处理新增和写入动作
				if event.Action != shared.FileActionCreate && event.Action != shared.FileActionWrite {
					return true
				}
				img, err := h.imgScanner.Lookup(ctx, event.RelPath)
				if err != nil || img == nil {
					// 文件可能已被快速删除，或不是支持的图片类型，跳过而不向客户端报错
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
		var allowedDirectoryIDs util.Set[scalar.ID]
		if filter != nil && len(filter.DirectoryID) > 0 {
			allowedDirectoryIDs = util.AddToSet(nil, filter.DirectoryID...)
		}
		imageFilter := h.imageFilterBuilder.Build(util.UnwrapPointer(filter))

		for event, err := range h.eventBus.SubscribeFileChanged(ctx) {
			if !func() bool {
				if err != nil {
					return yield(scalar.ToID(""), err)
				}
				// 只处理删除和重命名动作
				if event.Action != shared.FileActionRemove && event.Action != shared.FileActionRename {
					return true
				}
				// 事件级目录粗筛
				if allowedDirectoryIDs != nil && !allowedDirectoryIDs.Has(event.DirectoryID) {
					return true
				}
				// 尝试查找图片以获取其完整ID（虽然文件已删除，但可能还在缓存中）
				img, err := h.imgScanner.Lookup(ctx, event.RelPath)
				if err != nil || img == nil {
					return true
				}
				if !imageFilter(img) {
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

// TODO: 移动到　images 领域
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
	imgFilter := h.imageFilterBuilder.Build(filterBy)

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

	// 使用 ByDirection 解析分页选项
	dir, cursorStr, limit, err := pagination.ByDirection(options, true)
	if err != nil {
		return nil, err
	}

	var cursorTime time.Time
	if cursorStr != "" {
		cursorTime, err = pagination.TimeFromCursor(cursorStr)
		if err != nil {
			return nil, err
		}
	}

	// 收集并过滤图片
	var items []*shared.ImageDTO
	for img, scanErr := range h.imgScanner.Scan(ctx, relPath) {
		if scanErr != nil {
			return nil, scanErr
		}
		if !imgFilter(img) {
			continue
		}
		dto, factoryErr := h.imageDTOFactory.New(img)
		if factoryErr != nil {
			return nil, factoryErr
		}
		items = append(items, dto)
	}

	// 按照游标时间过滤图片（降序时保留更旧的图片，升序时保留更新的图片）
	if cursorStr != "" {
		var filtered []*shared.ImageDTO
		for _, item := range items {
			if dir == pagination.ODDescend {
				if item.ModTime.Before(cursorTime) {
					filtered = append(filtered, item)
				}
			} else {
				if item.ModTime.After(cursorTime) {
					filtered = append(filtered, item)
				}
			}
		}
		items = filtered
	}

	// 根据排序方向对图片列表进行稳定排序
	slices.SortFunc(items, func(a, b *shared.ImageDTO) int {
		if dir == pagination.ODDescend {
			if a.ModTime.After(b.ModTime) {
				return -1
			}
			if a.ModTime.Before(b.ModTime) {
				return 1
			}
			aStr := a.ID.String()
			bStr := b.ID.String()
			if aStr > bStr {
				return -1
			}
			if aStr < bStr {
				return 1
			}
			return 0
		} else {
			if a.ModTime.Before(b.ModTime) {
				return -1
			}
			if a.ModTime.After(b.ModTime) {
				return 1
			}
			aStr := a.ID.String()
			bStr := b.ID.String()
			if aStr < bStr {
				return -1
			}
			if aStr > bStr {
				return 1
			}
			return 0
		}
	})

	// 使用 ReverseWriter 自动处理 ODAscend 时的反向写入和反向 pageInfo
	var writer pagination.Writer[*shared.ImageDTO] = buf
	if dir == pagination.ODAscend {
		writer = pagination.NewReverseWriter[*shared.ImageDTO](buf)
	}

	// 裁剪并写入
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	writer.WriteHasNextPage(hasMore)
	writer.WriteHasPreviousPage(cursorStr != "")

	for _, item := range items {
		cursor := pagination.TimeToCursor(item.ModTime)
		err = writer.Write(item, cursor)
		if err != nil {
			return nil, err
		}
	}

	err = writer.Close()
	if err != nil {
		return nil, err
	}

	return buf.Value()
}

// TODO: 移动到　memo 领域
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
		memoFilter := h.memoFilterBuilder.Build(filterBy)
		for m, scanErr := range h.memoScanner.Scan(ctx, relPath) {
			if scanErr != nil {
				if !yield(nil, scanErr) {
					return
				}
				continue
			}
			if !memoFilter(m) {
				continue
			}
			dto := h.memoDTOFactory.New(m)
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

// MoveImages 移动当前目录中筛选匹配的图片及其配套文件至目标相对目录中（相对于当前目录）
func (h *Handler) MoveImages(
	ctx context.Context,
	directoryID scalar.ID,
	filterBy shared.ImageFilters,
	toDirectoryRelPath string,
) (movedCount int, targetAbsDir string, err error) {
	startTime := time.Now()

	// 还原当前目录相对路径
	relPath, err := directory.DecodeID(directoryID)
	if err != nil {
		return 0, "", err
	}

	h.logger.Info("will move images",
		zap.Stringer("directoryID", directoryID),
		zap.String("fromDirectory", relPath),
		zap.String("toDirectoryRelPath", toDirectoryRelPath),
	)

	movedCount, targetAbsDir, err = h.imgMover.Move(ctx, relPath, filterBy, toDirectoryRelPath)
	if err != nil {
		h.logger.Error("move images failed",
			zap.Stringer("directoryID", directoryID),
			zap.String("fromDirectory", relPath),
			zap.String("toDirectoryRelPath", toDirectoryRelPath),
			zap.Duration("duration", time.Since(startTime)),
			zap.Error(err),
		)
		return 0, "", err
	}

	h.logger.Info("did move images",
		zap.Stringer("directoryID", directoryID),
		zap.String("fromDirectory", relPath),
		zap.String("toDirectoryRelPath", toDirectoryRelPath),
		zap.Int("movedCount", movedCount),
		zap.String("targetAbsDir", targetAbsDir),
		zap.Duration("duration", time.Since(startTime)),
	)

	return movedCount, targetAbsDir, nil
}

// #endregion
