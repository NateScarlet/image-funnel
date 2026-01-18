package directory

import (
	"context"
	"main/internal/domain/directory"
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

func (h *Handler) GetDirectory(ctx context.Context, id string) (*DirectoryDTO, error) {
	if id == "" {
		id = directory.EncodeDirectoryID(".")
	}

	path, err := directory.DecodeDirectoryID(id)
	if err != nil {
		return nil, err
	}

	if err = h.scanner.ValidateDirectoryPath(path); err != nil {
		return nil, err
	}

	imageCount, subdirectoryCount, latestModTime, latestImagePath, ratingCounts, err := h.scanner.AnalyzeDirectory(path)
	if err != nil {
		return nil, err
	}

	var parentID string
	if path != "." {
		parentPath := filepath.Dir(path)
		if parentPath != "." {
			parentID = directory.EncodeDirectoryID(parentPath)
		} else {
			parentID = directory.EncodeDirectoryID(".")
		}
	}

	dirInfo := directory.NewDirectoryInfo(path, imageCount, subdirectoryCount, latestModTime, latestImagePath, ratingCounts)
	return h.dtoFactory.New(dirInfo, parentID, path == ".")
}

func (h *Handler) GetDirectories(ctx context.Context, parentID string) ([]*DirectoryDTO, error) {
	path, err := directory.DecodeDirectoryID(parentID)
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

	result := make([]*DirectoryDTO, len(dirs))
	for i, dir := range dirs {
		dirDTO, err := h.dtoFactory.New(dir, parentID, false)
		if err != nil {
			return nil, err
		}
		result[i] = dirDTO
	}

	return result, nil
}
