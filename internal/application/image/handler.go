package image

import (
	"context"
	"iter"
	"main/internal/apperror"
	"main/internal/domain/directory"
	"main/internal/domain/image"
	"main/internal/pagination"
	"main/internal/scalar"
	"main/internal/shared"
	"main/internal/util"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"go.uber.org/zap"
)

// ClipboardProvider 剪贴板操作接口，避免循环依赖
type ClipboardProvider interface {
	ReadCustomFormat(formatName string) (string, error)
	ReadHTMLFormat() (string, error)
	AddFiles(filePaths []string) error
	ListFormats() []string
}

// EventBus 文件变更事件总线接口，避免直接依赖 session 包造成循环导入
type EventBus interface {
	SubscribeFileChanged(ctx context.Context) iter.Seq2[*shared.FileChangedEvent, error]
}

type Handler struct {
	imageService       *image.Service
	eventBus           EventBus
	imgRepo            image.Repository
	imgMover           image.Mover
	imgTrasher         image.Trasher
	dirSvc             *directory.Service
	dtoFactory         *DTOFactory
	imageFilterBuilder *image.FilterBuilder
	logger             *zap.Logger
	rootDir            string
	imgFactory         *image.Factory
	clipboard          ClipboardProvider
}

func NewHandler(
	imageService *image.Service,
	eventBus EventBus,
	imgRepo image.Repository,
	imgMover image.Mover,
	imgTrasher image.Trasher,
	dirSvc *directory.Service,
	dtoFactory *DTOFactory,
	imageFilterBuilder *image.FilterBuilder,
	logger *zap.Logger,
	rootDir string,
	imgFactory *image.Factory,
	clipboard ClipboardProvider,
) *Handler {
	return &Handler{
		imageService:       imageService,
		eventBus:           eventBus,
		imgRepo:            imgRepo,
		imgMover:           imgMover,
		imgTrasher:         imgTrasher,
		dirSvc:             dirSvc,
		dtoFactory:         dtoFactory,
		imageFilterBuilder: imageFilterBuilder,
		logger:             logger,
		rootDir:            rootDir,
		imgFactory:         imgFactory,
		clipboard:          clipboard,
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

	workflow, err := ExtractComfyUIWorkflow(filepath.Join(h.rootDir, img.RelPath()))
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
				img, err := h.imgRepo.Get(ctx, event.RelPath)
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
	for img, scanErr := range h.imgRepo.Find(ctx, relPath) {
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

// MoveImages 移动当前目录中筛选匹配的图片及其配套文件至目标目录中
func (h *Handler) MoveImages(
	ctx context.Context,
	directoryID scalar.ID,
	filterBy shared.ImageFilters,
	toDirectory shared.PathInput,
) (movedCount int, targetAbsDir string, err error) {
	startTime := time.Now()

	dirInfo, err := h.dirSvc.GetDirectory(ctx, directoryID)
	if err != nil {
		return 0, "", err
	}
	relPath := dirInfo.RelPath()

	// 使用领域服务解析并校验输入路径
	targetRelPath, err := h.dirSvc.ResolvePathInput(ctx, relPath, toDirectory)
	if err != nil {
		return 0, "", err
	}

	h.logger.Info("will move images",
		zap.Stringer("directoryID", directoryID),
		zap.String("fromDirectory", relPath),
		zap.String("targetRelPath", targetRelPath),
	)

	movedCount, targetAbsDir, err = h.imgMover.Move(ctx, relPath, filterBy, targetRelPath)
	if err != nil {
		h.logger.Error("move images failed",
			zap.Stringer("directoryID", directoryID),
			zap.String("fromDirectory", relPath),
			zap.String("targetRelPath", targetRelPath),
			zap.Duration("duration", time.Since(startTime)),
			zap.Error(err),
		)
		return 0, "", err
	}

	h.logger.Info("did move images",
		zap.Stringer("directoryID", directoryID),
		zap.String("fromDirectory", relPath),
		zap.String("targetRelPath", targetRelPath),
		zap.Int("movedCount", movedCount),
		zap.String("targetAbsDir", targetAbsDir),
		zap.Duration("duration", time.Since(startTime)),
	)

	return movedCount, targetAbsDir, nil
}

// TrashImages 将符合条件的图片移至隐藏的暂存垃圾箱内，并返回生成的历史ID与文件数
func (h *Handler) TrashImages(
	ctx context.Context,
	directoryID scalar.ID,
	filterBy shared.ImageFilters,
) (historyId string, totalFileCount int, err error) {
	startTime := time.Now()

	dirInfo, err := h.dirSvc.GetDirectory(ctx, directoryID)
	if err != nil {
		return "", 0, err
	}
	relPath := dirInfo.RelPath()

	h.logger.Info("will trash images",
		zap.Stringer("directoryID", directoryID),
		zap.String("fromDirectory", relPath),
	)

	historyId, totalFileCount, err = h.imgTrasher.Trash(ctx, relPath, filterBy)
	if err != nil {
		h.logger.Error("trash images failed",
			zap.Stringer("directoryID", directoryID),
			zap.String("fromDirectory", relPath),
			zap.Duration("duration", time.Since(startTime)),
			zap.Error(err),
		)
		return "", 0, err
	}

	h.logger.Info("did trash images",
		zap.Stringer("directoryID", directoryID),
		zap.String("fromDirectory", relPath),
		zap.String("historyId", historyId),
		zap.Int("totalFileCount", totalFileCount),
		zap.Duration("duration", time.Since(startTime)),
	)

	return historyId, totalFileCount, nil
}

// UndoTrash 撤销指定的暂存垃圾箱移动操作，还原文件
func (h *Handler) UndoTrash(
	ctx context.Context,
	historyId string,
) (restoredCount int, err error) {
	startTime := time.Now()

	h.logger.Info("will undo trash", zap.String("historyId", historyId))

	restoredCount, err = h.imgTrasher.UndoTrash(ctx, historyId)
	if err != nil {
		h.logger.Error("undo trash failed",
			zap.String("historyId", historyId),
			zap.Duration("duration", time.Since(startTime)),
			zap.Error(err),
		)
		return 0, err
	}

	h.logger.Info("did undo trash",
		zap.String("historyId", historyId),
		zap.Int("restoredCount", restoredCount),
		zap.Duration("duration", time.Since(startTime)),
	)

	return restoredCount, nil
}

// EmptyTrash 手动清空早于指定保留期限的暂存记录，移入系统回收站
func (h *Handler) EmptyTrash(
	ctx context.Context,
	minAge scalar.Duration,
) (clearedCount int, err error) {
	startTime := time.Now()

	stdDuration, err := minAge.Standard()
	if err != nil {
		return 0, err
	}

	h.logger.Info("will empty trash", zap.Duration("minAge", stdDuration))

	clearedCount, err = h.imgTrasher.EmptyTrash(ctx, stdDuration)
	if err != nil {
		h.logger.Error("empty trash failed",
			zap.Duration("minAge", stdDuration),
			zap.Duration("duration", time.Since(startTime)),
			zap.Error(err),
		)
		return 0, err
	}

	h.logger.Info("did empty trash",
		zap.Duration("minAge", stdDuration),
		zap.Int("clearedCount", clearedCount),
		zap.Duration("duration", time.Since(startTime)),
	)

	return clearedCount, nil
}

// TrashHistory 获取垃圾暂存历史记录，支持游标分页
func (h *Handler) TrashHistory(
	ctx context.Context,
	first *int,
	after *string,
) (connection *shared.TrashHistoryConnectionDTO, err error) {
	if first == nil {
		defaultFirst := 100
		first = &defaultFirst
	}

	builder := pagination.NewConnectionBufferBuilder[*shared.TrashHistoryItemDTO, *shared.TrashHistoryEdgeDTO, *shared.TrashHistoryConnectionDTO]()
	buf := builder(
		func(item *shared.TrashHistoryItemDTO, cursor string) (*shared.TrashHistoryEdgeDTO, error) {
			return &shared.TrashHistoryEdgeDTO{
				Node:   item,
				Cursor: cursor,
			}, nil
		},
		func(edges []*shared.TrashHistoryEdgeDTO, pageInfo pagination.PageInfo) (*shared.TrashHistoryConnectionDTO, error) {
			var nodes = make([]*shared.TrashHistoryItemDTO, len(edges))
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
			return &shared.TrashHistoryConnectionDTO{
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

	filteredSeq := func(yield func(*shared.TrashHistoryItemDTO, error) bool) {
		historySeq := h.imgTrasher.FindTrashHistory(ctx)
		for item, err := range historySeq {
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			if !yield(item, nil) {
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

// AttachFileToClipboardResult 附加文件到剪贴板操作结果
type AttachFileToClipboardResult struct {
	Supported bool
}

// AttachFileToClipboard 将文件附加到系统剪贴板，通过随机数验证后附加文件对象
func (h *Handler) AttachFileToClipboard(
	ctx context.Context,
	paths []string,
	nonce string,
) (*AttachFileToClipboardResult, error) {
	startTime := time.Now()

	defer func() {
		h.logger.Debug("AttachFileToClipboard completed",
			zap.Strings("paths", paths),
			zap.Duration("duration", time.Since(startTime)),
		)
	}()

	// 1. 平台检测 (需求 4)
	// 如果非 Windows 平台，返回 supported: false 且不抛出错误
	if runtime.GOOS != "windows" {
		h.logger.Debug("Clipboard file attachment not supported on this platform", zap.String("goos", runtime.GOOS))
		return &AttachFileToClipboardResult{
			Supported: false,
		}, nil
	}

	if h.clipboard == nil {
		h.logger.Debug("Clipboard provider not configured")
		return &AttachFileToClipboardResult{
			Supported: false,
		}, nil
	}

	// 2. 验证剪贴板中的随机数：从 HTML 格式的 meta 标签中读取
	htmlContent, err := h.clipboard.ReadHTMLFormat()
	if err != nil {
		h.logger.Debug("Failed to read clipboard HTML", zap.Error(err))
		// 如果读不到剪贴板 HTML，也作为不支持处理而不抛出错误（可能由于跨设备或者权限不足）
		return &AttachFileToClipboardResult{
			Supported: false,
		}, nil
	}

	clipboardNonce := extractClipboardNonce(htmlContent)
	if clipboardNonce != nonce {
		// 调试：列出当前剪贴板中所有格式
		formats := h.clipboard.ListFormats()
		h.logger.Debug("Clipboard nonce mismatch (this might indicate client & server are running on different machines)",
			zap.String("expected", nonce),
			zap.String("got", clipboardNonce),
			zap.Strings("allFormats", formats))
		// 随机数验证失败时，由于这表明可能运行在不同的物理机器上，故不抛出错误，而是返回 supported: false
		return &AttachFileToClipboardResult{
			Supported: false,
		}, nil
	}

	// 将路径转换为绝对路径，并校验安全性
	var absPaths []string
	for _, p := range paths {
		var absPath string
		var relForValidation string

		if filepath.IsAbs(p) {
			absPath = p
			var err error
			relForValidation, err = filepath.Rel(h.rootDir, p)
			if err != nil {
				h.logger.Warn("Path validation failed", zap.String("path", p), zap.Error(err))
				return nil, apperror.New("PATH_INVALID", "Path validation failed", "路径校验失败")
			}
		} else {
			absPath = filepath.Join(h.rootDir, p)
			relForValidation = p
		}

		// 校验路径不逃逸根目录
		if err := util.EnsurePathInRoot(h.rootDir, relForValidation); err != nil {
			h.logger.Warn("Path validation failed", zap.String("path", p), zap.Error(err))
			return nil, apperror.New("PATH_INVALID", "Path validation failed", "路径校验失败")
		}
		absPaths = append(absPaths, absPath)
	}

	// 添加文件到剪贴板，保留已有数据
	err = h.clipboard.AddFiles(absPaths)
	if err != nil {
		h.logger.Error("Failed to add files to clipboard", zap.Error(err))
		return nil, apperror.New("CLIPBOARD_WRITE_FAILED", "Failed to add files to clipboard", "无法添加文件到剪贴板")
	}

	return &AttachFileToClipboardResult{
		Supported: true,
	}, nil
}

// clipboardTokenMetaName 剪贴板令牌在 HTML meta 标签中的 name 属性值
const clipboardTokenMetaName = "io.github.natescarlet.image-funnel.nonce"

// extractClipboardNonce 从剪贴板 HTML 格式中提取随机数
func extractClipboardNonce(htmlContent string) string {
	// 匹配 <meta name="io.github.natescarlet.image-funnel.nonce" content="..."/>
	pattern := regexp.MustCompile(
		`<meta\s+name="` + regexp.QuoteMeta(clipboardTokenMetaName) + `"\s+content="([^"]*)"`,
	)
	matches := pattern.FindStringSubmatch(htmlContent)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}
