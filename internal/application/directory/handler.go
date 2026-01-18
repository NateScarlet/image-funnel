package directory

import (
	"context"
	"main/internal/domain/directory"
)

type Handler struct {
	scanner directory.Scanner
}

func NewHandler(scanner directory.Scanner) *Handler {
	return &Handler{
		scanner: scanner,
	}
}

func (h *Handler) GetDirectory(ctx context.Context, path string) (*DirectoryDTO, error) {
	imageCount, subdirectoryCount, latestModTime, latestImagePath, ratingCounts, err := h.scanner.AnalyzeDirectory(path)
	if err != nil {
		return nil, err
	}

	return &DirectoryDTO{
		ID:                 path,
		Path:               path,
		ImageCount:         imageCount,
		SubdirectoryCount:  subdirectoryCount,
		LatestImageModTime: latestModTime,
		LatestImagePath:    latestImagePath,
		RatingCounts:       ratingCounts,
	}, nil
}

func (h *Handler) GetDirectories(ctx context.Context, relPath string) ([]*DirectoryDTO, error) {
	dirs, err := h.scanner.ScanDirectories(relPath)
	if err != nil {
		return nil, err
	}

	result := make([]*DirectoryDTO, len(dirs))
	for i, dir := range dirs {
		result[i] = &DirectoryDTO{
			ID:                 dir.Path(),
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
