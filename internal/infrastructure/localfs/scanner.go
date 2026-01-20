package localfs

import (
	"context"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	appimage "main/internal/application/image"
	"main/internal/domain/directory"
	domainimage "main/internal/domain/image"
	"main/internal/domain/metadata"
	"main/internal/iterator"
	"main/internal/util"
)

type Scanner struct {
	rootDir   string
	xmpRepo   metadata.Repository
	processor appimage.Processor
}

func NewScanner(rootDir string, xmpRepo metadata.Repository, processor appimage.Processor) *Scanner {
	return &Scanner{
		rootDir:   rootDir,
		xmpRepo:   xmpRepo,
		processor: processor,
	}
}

func (s *Scanner) Scan(relPath string) iter.Seq2[*domainimage.Image, error] {
	return func(yield func(*domainimage.Image, error) bool) {
		absPath := filepath.Join(s.rootDir, relPath)
		entries, err := os.ReadDir(absPath)
		if err != nil {
			yield(nil, fmt.Errorf("failed to read directory: %w", err))
			return
		}

		ctx := context.Background()
		limit := runtime.NumCPU()

		iterator.ParallelConcatMapTo2(
			ctx,
			limit,
			slices.Values(entries),
			yield,
		)(
			func(ctx context.Context, yield func(*domainimage.Image, error) bool, entry os.DirEntry) bool {
				if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
					return true
				}

				if !s.isSupportedImage(entry.Name()) {
					return true
				}

				path := filepath.Join(absPath, entry.Name())
				info, err := entry.Info()
				if err != nil {
					return yield(nil, err)
				}

				var xmpData *metadata.XMPData

				xmpData, err = s.xmpRepo.Read(path)
				if err != nil {
					return yield(nil, err)
				}

				width, height := 0, 0
				if s.processor != nil {
					meta, err := s.processor.Meta(ctx, path)
					if err == nil {
						width, height = meta.Width, meta.Height
					}
				}

				return yield(domainimage.NewImageFromPath(
					entry.Name(),
					path,
					info.Size(),
					info.ModTime(),
					xmpData,
					width,
					height,
				), nil)
			},
		)
	}
}

func (s *Scanner) ScanDirectories(relPath string) iter.Seq2[*directory.DirectoryInfo, error] {
	return func(yield func(*directory.DirectoryInfo, error) bool) {
		if relPath != "" {
			if err := util.EnsurePathInRoot(s.rootDir, relPath); err != nil {
				yield(nil, err)
				return
			}
		}

		absPath := filepath.Join(s.rootDir, relPath)
		entries, err := os.ReadDir(absPath)
		if err != nil {
			yield(nil, fmt.Errorf("failed to read directory: %w", err))
			return
		}

		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}

			subRelPath := filepath.Join(relPath, entry.Name())
			dirInfo := directory.NewDirectoryInfo(subRelPath)

			if !yield(dirInfo, nil) {
				break
			}
		}
	}
}

func (s *Scanner) AnalyzeDirectory(ctx context.Context, relPath string) (*directory.DirectoryStats, error) {
	if err := util.EnsurePathInRoot(s.rootDir, relPath); err != nil {
		return nil, err
	}

	absPath := filepath.Join(s.rootDir, relPath)
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, err
	}

	imageCount := 0
	subdirectoryCount := 0
	var latestImage *domainimage.Image
	ratingCounts := make(map[int]int)

	for _, entry := range entries {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		if entry.IsDir() {
			subdirectoryCount++
			continue
		}

		if !s.isSupportedImage(entry.Name()) {
			continue
		}

		imageCount++
		info, err := entry.Info()
		if err != nil {
			continue
		}

		imagePath := filepath.Join(absPath, entry.Name())

		var xmpData *metadata.XMPData

		xmpData, err = s.xmpRepo.Read(imagePath)
		if err != nil {
			xmpData = nil
		}

		width, height := 0, 0
		if s.processor != nil {
			meta, err := s.processor.Meta(ctx, imagePath)
			if err == nil {
				width, height = meta.Width, meta.Height
			}
		}

		img := domainimage.NewImageFromPath(
			entry.Name(),
			imagePath,
			info.Size(),
			info.ModTime(),
			xmpData,
			width,
			height,
		)

		if latestImage == nil || info.ModTime().After(latestImage.ModTime()) {
			latestImage = img
		}
		ratingCounts[img.Rating()]++
	}

	return directory.NewDirectoryStats(imageCount, subdirectoryCount, latestImage, ratingCounts), nil
}

func (s *Scanner) isSupportedImage(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" || ext == ".avif"
}

var _ directory.Scanner = (*Scanner)(nil)
