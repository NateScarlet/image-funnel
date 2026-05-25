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
	"main/internal/iterator"
)

// ImageScanner 负责本地文件系统上的图片检索与基本信息解析
type ImageScanner struct {
	rootDir      string
	imageFactory *domainimage.Factory
	dirRepo      directory.Repository
}

// NewImageScanner 创建图片扫描实现实例
func NewImageScanner(rootDir string, imageFactory *domainimage.Factory, dirRepo directory.Repository) *ImageScanner {
	return &ImageScanner{
		rootDir:      rootDir,
		imageFactory: imageFactory,
		dirRepo:      dirRepo,
	}
}

// Scan 迭代遍历指定相对路径下的物理文件并转为领域层 Image 对象
func (s *ImageScanner) Scan(ctx context.Context, relPath string) iter.Seq2[*domainimage.Image, error] {
	return func(yield func(*domainimage.Image, error) bool) {
		absPath := filepath.Join(s.rootDir, relPath)
		entries, err := os.ReadDir(absPath)
		if err != nil {
			yield(nil, fmt.Errorf("failed to read directory: %w", err))
			return
		}

		// 通过仓库获取合法的目录对象，由其生成目录 ID
		dir, err := s.dirRepo.Get(ctx, relPath)
		if err != nil {
			yield(nil, err)
			return
		}
		directoryID := dir.ID()

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
					return true // 不支持的格式或跳过
				}

				return yield(img, nil)
			},
		)
	}
}

// Lookup 还原出单个图片相对路径的领域 Image 对象
func (s *ImageScanner) Lookup(ctx context.Context, relPath string) (*domainimage.Image, error) {
	dirRelPath := filepath.Dir(relPath)
	if dirRelPath == "." {
		dirRelPath = ""
	}
	dir, err := s.dirRepo.Get(ctx, dirRelPath)
	if err != nil {
		return nil, err
	}
	return s.imageFactory.Create(ctx, relPath, s.rootDir, dir.ID())
}

var _ domainimage.Scanner = (*ImageScanner)(nil)