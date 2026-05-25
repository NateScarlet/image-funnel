package localfs

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"main/internal/domain/directory"
	domainimage "main/internal/domain/image"
	"main/internal/iterator"
	"main/internal/util"
)

// DirectoryAnalyzer 负责收集并汇总目标物理目录中包含的图片总数、等级统计及子目录数量等
type DirectoryAnalyzer struct {
	rootDir      string
	imageFactory *domainimage.Factory
	dirRepo      directory.Repository
}

// NewDirectoryAnalyzer 创建目录分析实现实例
func NewDirectoryAnalyzer(rootDir string, imageFactory *domainimage.Factory, dirRepo directory.Repository) *DirectoryAnalyzer {
	return &DirectoryAnalyzer{
		rootDir:      rootDir,
		imageFactory: imageFactory,
		dirRepo:      dirRepo,
	}
}

// Analyze 遍历物理目录并汇总得到统计数据
func (s *DirectoryAnalyzer) Analyze(ctx context.Context, relPath string) (*directory.Stats, error) {
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

	// 通过仓库获取合法的目录对象，由其生成目录 ID
	dir, err := s.dirRepo.Get(ctx, relPath)
	if err != nil {
		return nil, err
	}
	directoryID := dir.ID()

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

	return directory.NewStats(imageCount, subdirectoryCount, latestImage, ratingCounts), nil
}

var _ directory.Analyzer = (*DirectoryAnalyzer)(nil)
