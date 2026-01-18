package directory

import (
	"context"
	"main/internal/domain/directory"
	"path/filepath"
)

type Handler struct {
	scanner directory.Scanner
}

func NewHandler(scanner directory.Scanner) *Handler {
	return &Handler{
		scanner: scanner,
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

	return &DirectoryDTO{
		ID:                 id,
		ParentID:           parentID,
		Path:               path,
		ImageCount:         imageCount,
		SubdirectoryCount:  subdirectoryCount,
		LatestImageModTime: latestModTime,
		LatestImagePath:    latestImagePath,
		RatingCounts:       ratingCounts,
	}, nil
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
		result[i] = &DirectoryDTO{
			ID:                 directory.EncodeDirectoryID(dir.Path()),
			ParentID:           parentID,
			Path:               dir.Path(),
			ImageCount:         dir.ImageCount(),
			SubdirectoryCount:  dir.SubdirectoryCount(),
			LatestImageModTime: dir.LatestImageModTime(),
			LatestImagePath:    dir.LatestImagePath(),
			RatingCounts:       dir.RatingCounts(),
		}
	}

	return result, nil
}
