package directory

import (
	"context"
	"main/internal/domain/directory"
	"main/internal/scalar"
	"main/internal/shared"
	"path/filepath"
)

type Handler struct {
	scanner    directory.Scanner
	dtoFactory *DirectoryDTOFactory
}

func NewHandler(scanner directory.Scanner) *Handler {
	return &Handler{
		scanner:    scanner,
		dtoFactory: NewDirectoryDTOFactory(),
	}
}

func (h *Handler) GetDirectory(ctx context.Context, id scalar.ID) (*shared.DirectoryDTO, error) {
	if id.String() == "" {
		id = directory.EncodeID(".")
	}

	path, err := directory.DecodeID(id)
	if err != nil {
		return nil, err
	}

	if err = h.scanner.ValidateDirectoryPath(path); err != nil {
		return nil, err
	}

	imageCount, subdirectoryCount, latestImage, ratingCounts, err := h.scanner.AnalyzeDirectory(path)
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

	dirInfo := directory.NewDirectoryInfo(path, imageCount, subdirectoryCount, latestImage, ratingCounts)
	return h.dtoFactory.New(dirInfo, parentID, path == ".")
}

func (h *Handler) GetDirectories(ctx context.Context, parentID scalar.ID) ([]*shared.DirectoryDTO, error) {
	path, err := directory.DecodeID(parentID)
	if err != nil {
		return nil, err
	}

	if err = h.scanner.ValidateDirectoryPath(path); err != nil {
		return nil, err
	}

	dirs, err := h.scanner.ScanDirectories(path)
	if err != nil {
		return nil, err
	}

	result := make([]*shared.DirectoryDTO, len(dirs))
	for i, dir := range dirs {
		dirDTO, err := h.dtoFactory.New(dir, parentID, false)
		if err != nil {
			return nil, err
		}
		result[i] = dirDTO
	}

	return result, nil
}
