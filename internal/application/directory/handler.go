package directory

import (
	"context"
	"iter"
	appimage "main/internal/application/image"
	appsession "main/internal/application/session"
	"main/internal/domain/directory"
	"main/internal/scalar"
	"main/internal/shared"
	"path/filepath"
)

// Handler 目录应用层处理器
type Handler struct {
	scanner    directory.Scanner
	eventBus   appsession.EventBus
	dtoFactory *DirectoryDTOFactory

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
		scanner:       scanner,
		eventBus:      eventBus,
		dtoFactory:    NewDirectoryDTOFactory(imageDTOFactory),
		filterBuilder: directory.NewFilterBuilder(),
		repo:          repo,
	}
}

// Directory 查询目录信息
func (h *Handler) Directory(ctx context.Context, id scalar.ID) (*shared.DirectoryDTO, error) {
	dirInfo, err := h.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	path := dirInfo.Path()
	var parentID scalar.ID
	if path != "." {
		parentPath := filepath.Dir(path)
		if parentPath != "." {
			parentID = directory.EncodeID(parentPath)
		} else {
			parentID = directory.EncodeID(".")
		}
	}

	return h.dtoFactory.New(dirInfo, parentID, path == "."), nil
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
