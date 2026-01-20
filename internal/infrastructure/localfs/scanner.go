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
					return true
				}

				var xmpData *metadata.XMPData
				if s.xmpExists(path) {
					xmpData, err = s.xmpRepo.Read(path)
					if err != nil {
						xmpData = nil
					}
				}

				width, height := 0, 0
				if s.processor != nil {
					meta, err := s.processor.Meta(ctx, path)
					if err == nil {
						width, height = meta.Width, meta.Height
					}
				}

				img := domainimage.NewImageFromPath(
					entry.Name(),
					path,
					info.Size(),
					info.ModTime(),
					xmpData,
					width,
					height,
				)

				return yield(img, nil)
			},
		)
	}
}

func (s *Scanner) ScanDirectories(relPath string) iter.Seq2[*directory.DirectoryInfo, error] {
	return func(yield func(*directory.DirectoryInfo, error) bool) {
		if relPath != "" {
			if err := s.ValidateDirectoryPath(relPath); err != nil {
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
	if err := s.ValidateDirectoryPath(relPath); err != nil {
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
		if s.xmpExists(imagePath) {
			xmpData, err = s.xmpRepo.Read(imagePath)
			if err != nil {
				xmpData = nil
			}
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

		if s.xmpExists(imagePath) {
			xmpData, err := s.xmpRepo.Read(imagePath)
			if err == nil {
				ratingCounts[xmpData.Rating()]++
			} else {
				ratingCounts[0]++
			}
		} else {
			ratingCounts[0]++
		}
	}

	stats := directory.NewDirectoryStats(imageCount, subdirectoryCount, latestImage, ratingCounts)

	return stats, nil
}

func (s *Scanner) ValidateDirectoryPath(relPath string) error {
	if relPath == "" {
		relPath = "."
	}

	if strings.Contains(relPath, "..") {
		return fmt.Errorf("invalid path: contains parent directory reference")
	}

	if strings.Contains(relPath, ":") {
		return fmt.Errorf("invalid path: contains drive letter")
	}

	if filepath.IsAbs(relPath) {
		return fmt.Errorf("invalid path: absolute path not allowed")
	}

	absPath := filepath.Join(s.rootDir, relPath)
	cleanAbsPath := filepath.Clean(absPath)
	cleanRootDir := filepath.Clean(s.rootDir)

	relFromRoot, err := filepath.Rel(cleanRootDir, cleanAbsPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	if strings.HasPrefix(relFromRoot, "..") {
		return fmt.Errorf("invalid path: escapes root directory")
	}

	_, err = os.Stat(cleanAbsPath)
	if err != nil {
		return fmt.Errorf("directory does not exist: %w", err)
	}

	return nil
}

func (s *Scanner) xmpExists(imagePath string) bool {
	_, err := os.Stat(imagePath + ".xmp")
	return err == nil
}

func (s *Scanner) isSupportedImage(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" || ext == ".avif"
}

var _ directory.Scanner = (*Scanner)(nil)
