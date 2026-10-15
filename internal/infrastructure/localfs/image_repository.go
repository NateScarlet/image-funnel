package localfs

import (
	"context"
	"fmt"
	"iter"
	"main/internal/domain/directory"
	"main/internal/domain/image"
	"main/internal/domain/relpath"
	"main/internal/iterator"
	"os"
	"runtime"
	"slices"
	"strings"
)

type ImageRepository struct {
	rootDir string
	factory *image.Factory
	dirRepo directory.Repository
}

func NewImageRepository(rootDir string, factory *image.Factory, dirRepo directory.Repository) *ImageRepository {
	return &ImageRepository{
		rootDir: rootDir,
		factory: factory,
		dirRepo: dirRepo,
	}
}

func (r *ImageRepository) Get(ctx context.Context, relPath relpath.Path) (*image.Image, error) {
	absPath := relPath.Abs(r.rootDir)
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}

	dir, err := r.dirRepo.Get(ctx, relPath.Dir())
	if err != nil {
		return nil, err
	}

	return r.factory.CreateFromInfo(ctx, info, relPath, dir.ID())
}

func (r *ImageRepository) Find(ctx context.Context, relPath relpath.Path) iter.Seq2[*image.Image, error] {
	return func(yield func(*image.Image, error) bool) {
		entries, err := os.ReadDir(relPath.Abs(r.rootDir))
		if err != nil {
			yield(nil, fmt.Errorf("failed to read directory: %w", err))
			return
		}

		dir, err := r.dirRepo.Get(ctx, relPath)
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
			func(ctx context.Context, yield func(*image.Image, error) bool, entry os.DirEntry) bool {
				if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
					return true
				}

				// 文件项来自文件系统遍历，在此处构造根相对路径（出生点之一）并围栏
				fileRelPath, err := relPath.Join(r.rootDir, entry.Name())
				if err != nil {
					return yield(nil, err)
				}
				info, err := entry.Info()
				if err != nil {
					return yield(nil, err)
				}

				img, err := r.factory.CreateFromInfo(ctx, info, fileRelPath, directoryID)
				if err != nil {
					return yield(nil, err)
				}
				if img == nil {
					return true
				}

				return yield(img, nil)
			},
		)
	}
}

var _ image.Repository = (*ImageRepository)(nil)
