package graph

import (
	"context"
	"path/filepath"
	"sort"

	"main/internal/scanner"
)

func (r *Resolver) Directories(ctx context.Context, path string) ([]*Directory, error) {
	s := scanner.NewScanner(r.RootDir)

	dirs, err := s.ScanDirectories(path)
	if err != nil {
		return nil, err
	}

	sort.Slice(dirs, func(i, j int) bool {
		return dirs[i].LatestImageModTime.Before(dirs[j].LatestImageModTime)
	})

	var result []*Directory
	for _, dir := range dirs {
		latestImagePath := ""
		var latestImageUrl *string
		if dir.LatestImagePath != "" {
			relPath, err := filepath.Rel(r.RootDir, dir.LatestImagePath)
			if err == nil {
				latestImagePath = relPath
				url, err := r.Signer.GenerateSignedURL(dir.LatestImagePath)
				if err == nil {
					latestImageUrl = &url
				}
			}
		}

		var ratingCounts []*RatingCount
		for rating, count := range dir.RatingCounts {
			ratingCounts = append(ratingCounts, &RatingCount{
				Rating: rating,
				Count:  count,
			})
		}

		result = append(result, &Directory{
			ID:                 dir.Path,
			Path:               dir.Path,
			ImageCount:         dir.ImageCount,
			SubdirectoryCount:  dir.SubdirectoryCount,
			LatestImageModTime: dir.LatestImageModTime,
			LatestImagePath:    &latestImagePath,
			LatestImageURL:     latestImageUrl,
			RatingCounts:       ratingCounts,
		})
	}

	return result, nil
}
