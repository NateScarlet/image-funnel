package directory

import (
	"context"
	appimage "main/internal/application/image"
	"main/internal/domain/directory"
	"main/internal/scalar"
	"main/internal/shared"
	"path/filepath"
)

type Handler struct {
	scanner    directory.Scanner
	dtoFactory *DirectoryDTOFactory
}

func NewHandler(scanner directory.Scanner, imageDTOFactory *appimage.ImageDTOFactory) *Handler {
	return &Handler{
		scanner:    scanner,
		dtoFactory: NewDirectoryDTOFactory(imageDTOFactory),
	}
}

func (h *Handler) Directory(ctx context.Context, id scalar.ID) (*shared.DirectoryDTO, error) {
	if id.String() == "" {
		id = directory.EncodeID(".")
	}

	path, err := directory.DecodeID(id)
	if err != nil {
		return nil, err
	}

	var parentID scalar.ID
	if path != "." {
		parentPath := filepath.Dir(path)
		if parentPath != "." {
			parentID = directory.EncodeID(parentPath)
		} else {
			parentID = directory.EncodeID(".")
		}
	}

	dirInfo := directory.NewDirectoryInfo(path)
	return h.dtoFactory.New(dirInfo, parentID, path == "."), nil
}

func (h *Handler) DirectoryStats(ctx context.Context, id scalar.ID) (*shared.DirectoryStatsDTO, error) {
	if id.String() == "" {
		id = directory.EncodeID(".")
	}

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

func (h *Handler) Directories(ctx context.Context, parentID scalar.ID) ([]*shared.DirectoryDTO, error) {
	path, err := directory.DecodeID(parentID)
	if err != nil {
		return nil, err
	}

	var result []*shared.DirectoryDTO
	for dir, err := range h.scanner.ScanDirectories(path) {
		if err != nil {
			return nil, err
		}
		dirDTO := h.dtoFactory.New(dir, parentID, false)
		result = append(result, dirDTO)
	}

	return result, nil
}
