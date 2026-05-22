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

	"main/internal/domain/directory"
	domainimage "main/internal/domain/image"
	"main/internal/domain/memo"
	"main/internal/iterator"
	"main/internal/scalar"
	"main/internal/util"
)

type Scanner struct {
	rootDir      string
	imageFactory *domainimage.Factory
	dirRepo      directory.Repository
}

func NewScanner(rootDir string, imageFactory *domainimage.Factory, dirRepo directory.Repository) *Scanner {
	return &Scanner{
		rootDir:      rootDir,
		imageFactory: imageFactory,
		dirRepo:      dirRepo,
	}
}

func (s *Scanner) Scan(ctx context.Context, relPath string) iter.Seq2[*domainimage.Image, error] {
	return func(yield func(*domainimage.Image, error) bool) {
		absPath := filepath.Join(s.rootDir, relPath)
		entries, err := os.ReadDir(absPath)
		if err != nil {
			yield(nil, fmt.Errorf("failed to read directory: %w", err))
			return
		}

		// 计算当前目录的ID
		directoryID := directory.EncodeID(relPath)

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

				absFilePath := filepath.Join(absPath, entry.Name())
				info, err := entry.Info()
				if err != nil {
					return yield(nil, err)
				}

				img, err := s.imageFactory.CreateFromInfo(ctx, info, absFilePath, directoryID)
				if err != nil {
					return yield(nil, err)
				}
				if img == nil {
					return true // Not supported or skipped
				}

				return yield(img, nil)
			},
		)
	}
}

func (s *Scanner) ScanDirectories(ctx context.Context, relPath string) iter.Seq2[*directory.Directory, error] {
	return func(yield func(*directory.Directory, error) bool) {
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
			if ctx.Err() != nil {
				yield(nil, ctx.Err())
				return
			}
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}

			subRelPath := filepath.Join(relPath, entry.Name())
			dirInfo, err := s.dirRepo.GetByRelPath(ctx, subRelPath)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

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

	subdirectoryCount := 0
	var files []os.DirEntry

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if entry.IsDir() {
			subdirectoryCount++
			continue
		}
		files = append(files, entry)
	}

	imageCount := 0
	var latestImage *domainimage.Image
	ratingCounts := make(map[int]int)

	// 计算当前目录的ID
	directoryID := directory.EncodeID(relPath)

	limit := runtime.NumCPU()

	iterator.ParallelConcatMapTo2(
		ctx,
		limit,
		slices.Values(files),
		func(img *domainimage.Image, err error) bool {
			if err != nil || img == nil {
				return true
			}
			imageCount++
			if latestImage == nil || img.ModTime().After(latestImage.ModTime()) {
				latestImage = img
			}
			ratingCounts[img.Rating()]++
			return true
		},
	)(
		func(ctx context.Context, yield func(*domainimage.Image, error) bool, entry os.DirEntry) bool {
			if ctx.Err() != nil {
				return false
			}

			info, err := entry.Info()
			if err != nil {
				return yield(nil, err)
			}

			imagePath := filepath.Join(absPath, entry.Name())
			img, err := s.imageFactory.CreateFromInfo(ctx, info, imagePath, directoryID)
			return yield(img, err)
		},
	)

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	return directory.NewDirectoryStats(imageCount, subdirectoryCount, latestImage, ratingCounts), nil
}

func (s *Scanner) LookupImage(ctx context.Context, relPath string) (*domainimage.Image, error) {
	// 计算目录ID
	dirRelPath := filepath.Dir(relPath)
	if dirRelPath == "." {
		dirRelPath = ""
	}
	directoryID := directory.EncodeID(dirRelPath)
	return s.imageFactory.Create(ctx, relPath, s.rootDir, directoryID)
}

// ScanMemos 扫描物理目录下的备忘录
func (s *Scanner) ScanMemos(ctx context.Context, relPath string) iter.Seq2[*memo.Memo, error] {
	return func(yield func(*memo.Memo, error) bool) {
		absPath := filepath.Join(s.rootDir, relPath)
		entries, err := os.ReadDir(absPath)
		if err != nil {
			yield(nil, fmt.Errorf("failed to read directory: %w", err))
			return
		}

		for _, entry := range entries {
			if ctx.Err() != nil {
				yield(nil, ctx.Err())
				return
			}
			// 过滤掉目录和隐藏文件
			if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			// 过滤非 .md 结尾文件
			if strings.ToLower(filepath.Ext(entry.Name())) != ".md" {
				continue
			}

			// 推导关联图片相对路径以生成最匹配的 Memo ID
			baseName := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			supportedExtensions := []string{".jpg", ".jpeg", ".png", ".webp", ".avif"}
			imageFound := false
			imageRelPath := ""
			for _, ext := range supportedExtensions {
				imageFilename := baseName + ext
				imageAbsPath := filepath.Join(absPath, imageFilename)
				if _, err := os.Stat(imageAbsPath); err == nil {
					imageFound = true
					imageRelPath = filepath.Join(relPath, imageFilename)
					break
				}
			}

			var memoID scalar.ID
			if imageFound {
				memoID = memo.EncodeID(imageRelPath)
			} else {
				memoID = memo.EncodeID(filepath.Join(relPath, entry.Name()))
			}

			absFilePath := filepath.Join(absPath, entry.Name())
			contentBytes, err := os.ReadFile(absFilePath)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			m := memo.NewMemo(memoID, absFilePath, string(contentBytes))
			if !yield(m, nil) {
				return
			}
		}
	}
}

var _ directory.Scanner = (*Scanner)(nil)
