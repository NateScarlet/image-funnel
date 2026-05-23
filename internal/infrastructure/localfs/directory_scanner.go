package localfs

import (
	"context"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"strings"

	"main/internal/domain/directory"
	"main/internal/util"
)

// DirectoryScanner 负责遍历与检索直接下属物理子目录的实现
type DirectoryScanner struct {
	rootDir string
	dirRepo directory.Repository
}

// NewDirectoryScanner 创建目录扫描实现实例
func NewDirectoryScanner(rootDir string, dirRepo directory.Repository) *DirectoryScanner {
	return &DirectoryScanner{
		rootDir: rootDir,
		dirRepo: dirRepo,
	}
}

// Scan 扫描指定目录下直接一级的子目录
func (s *DirectoryScanner) Scan(ctx context.Context, relPath string) iter.Seq2[*directory.Directory, error] {
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

var _ directory.Scanner = (*DirectoryScanner)(nil)
