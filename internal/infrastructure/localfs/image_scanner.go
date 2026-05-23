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
}

// NewImageScanner 创建图片扫描实现实例
func NewImageScanner(rootDir string, imageFactory *domainimage.Factory) *ImageScanner {
	return &ImageScanner{
		rootDir:      rootDir,
		imageFactory: imageFactory,
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
					return true // 不支持的格式或跳过
				}

				return yield(img, nil)
			},
		)
	}
}

// Lookup 还原出单个图片相对路径的领域 Image 对象
func (s *ImageScanner) Lookup(ctx context.Context, relPath string) (*domainimage.Image, error) {
	// 计算目录ID
	dirRelPath := filepath.Dir(relPath)
	if dirRelPath == "." {
		dirRelPath = ""
	}
	directoryID := directory.EncodeID(dirRelPath)
	return s.imageFactory.Create(ctx, relPath, s.rootDir, directoryID)
}

var _ domainimage.Scanner = (*ImageScanner)(nil)
