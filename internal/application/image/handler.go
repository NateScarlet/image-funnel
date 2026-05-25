package image

import (
	"context"
	"iter"
	"main/internal/domain/directory"
	"main/internal/domain/image"
	"main/internal/pagination"
	"main/internal/scalar"
	"main/internal/shared"
	"main/internal/util"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"go.uber.org/zap"
)

// EventBus 文件变更事件总线接口，避免直接依赖 session 包造成循环导入
type EventBus interface {
	SubscribeFileChanged(ctx context.Context) iter.Seq2[*shared.FileChangedEvent, error]
}

type Handler struct {
	imageService       *image.Service
	eventBus           EventBus
	imgScanner         image.Scanner
	imgMover           image.Mover
	dirSvc             *directory.Service
	dtoFactory         *DTOFactory
	imageFilterBuilder *image.FilterBuilder
	logger             *zap.Logger
}

func NewHandler(
	imageService *image.Service,
	eventBus EventBus,
	imgScanner image.Scanner,
	imgMover image.Mover,
	dirSvc *directory.Service,
	dtoFactory *DTOFactory,
	imageFilterBuilder *image.FilterBuilder,
	logger *zap.Logger,
) *Handler {
	return &Handler{
		imageService:       imageService,
		eventBus:           eventBus,
		imgScanner:         imgScanner,
		imgMover:           imgMover,
		dirSvc:             dirSvc,
		dtoFactory:         dtoFactory,
		imageFilterBuilder: imageFilterBuilder,
		logger:             logger,
	}
}

// UpdateImageMetadata 更新单个图片的元数据（评星和颜色标签），操作即时写入 XMP 伴随文件
func (h *Handler) UpdateImageMetadata(
	ctx context.Context,
	id scalar.ID,
	rating *int,
	label *string,
) (err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("update image metadata failed",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did update image metadata",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	return h.imageService.UpdateImageMetadata(ctx, id, rating, label)
}

// Image 通过 ID 获取图片
func (h *Handler) Image(
	ctx context.Context,
	id scalar.ID,
) (*shared.ImageDTO, error) {
	img, err := h.imageService.GetImage(ctx, id)
	if err != nil {
		return nil, err
	}
	if img == nil {
		return nil, nil
	}
	return h.dtoFactory.New(img)
}

// ComfyUIWorkflow 通过图片 ID 获取 ComfyUI 工作流
func (h *Handler) ComfyUIWorkflow(
	ctx context.Context,
	id scalar.ID,
) (_ *string, err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("get ComfyUI workflow failed",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
				zap.Any("err", err),
			)
		} else {
			h.logger.Debug("did get ComfyUI workflow",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	img, err := h.imageService.GetImage(ctx, id)
	if err != nil {
		return nil, err
	}

	ext := strings.ToLower(filepath.Ext(img.Filename()))
	if ext != ".png" {
		return nil, nil
	}

	workflow, err := ExtractComfyUIWorkflow(img.AbsPath())
	if err != nil {
		return nil, err
	}

	return workflow, nil
}

// #region 图片变更订阅

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
				if event.Action != shared.FileActionCreate && event.Action != shared.FileActionWrite {
					return true
				}
				img, err := h.imgScanner.Lookup(ctx, event.RelPath)
				if err != nil || img == nil {
					return true
				}
				if !imageFilter(img) {
					return true
				}
				dto, err := h.dtoFactory.New(img)
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
				if event.Action != shared.FileActionRemove && event.Action != shared.FileActionRename {
					return true
				}
				if allowedDirectoryIDs != nil && !allowedDirectoryIDs.Has(event.DirectoryID) {
					return true
				}
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

// #endregion

// #region 图片过滤与分页查询

// Images 获取目录下的图片列表，支持过滤与基于 Relay 规范的游标分页
func (h *Handler) Images(
	ctx context.Context,
	id scalar.ID,
	filterBy shared.ImageFilters,
	first *int,
	after *string,
) (connection *shared.ImageConnectionDTO, err error) {
	if first == nil {
		defaultFirst := 100
		first = &defaultFirst
	}

	dirInfo, err := h.dirSvc.GetDirectory(ctx, id)
	if err != nil {
		return nil, err
	}

	imgFilter := h.imageFilterBuilder.Build(filterBy)

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

	options := pagination.OptionFromInput(after, nil, first, nil)

	relPath := dirInfo.RelPath()

	direction, cursorStr, limit, err := pagination.ByDirection(options, true)
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

	var items []*shared.ImageDTO
	for img, scanErr := range h.imgScanner.Scan(ctx, relPath) {
		if scanErr != nil {
			return nil, scanErr
		}
		if !imgFilter(img) {
			continue
		}
		dto, factoryErr := h.dtoFactory.New(img)
		if factoryErr != nil {
			return nil, factoryErr
		}
		items = append(items, dto)
	}

	if cursorStr != "" {
		var filtered []*shared.ImageDTO
		for _, item := range items {
			if direction == pagination.ODDescend {
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

	slices.SortFunc(items, func(a, b *shared.ImageDTO) int {
		if direction == pagination.ODDescend {
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

	var writer pagination.Writer[*shared.ImageDTO] = buf
	if direction == pagination.ODAscend {
		writer = pagination.NewReverseWriter[*shared.ImageDTO](buf)
	}

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

// #endregion

// MoveImages 移动当前目录中筛选匹配的图片及其配套文件至目标相对目录中（相对于当前目录）
func (h *Handler) MoveImages(
	ctx context.Context,
	directoryID scalar.ID,
	filterBy shared.ImageFilters,
	toDirectoryRelPath string,
) (movedCount int, targetAbsDir string, err error) {
	startTime := time.Now()

	dirInfo, err := h.dirSvc.GetDirectory(ctx, directoryID)
	if err != nil {
		return 0, "", err
	}
	relPath := dirInfo.RelPath()

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