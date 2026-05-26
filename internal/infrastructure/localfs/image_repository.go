package localfs

import (
	"context"
	"fmt"
	"iter"
	"main/internal/domain/directory"
	"main/internal/domain/image"
	"main/internal/iterator"
	"os"
	"path/filepath"
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

func (r *ImageRepository) Get(ctx context.Context, absPath string) (*image.Image, error) {
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}

	relPath, err := filepath.Rel(r.rootDir, absPath)
	if err != nil {
		return nil, err
	}
	dirRelPath := filepath.Dir(relPath)
	if dirRelPath == "." {
		dirRelPath = ""
	}

	dir, err := r.dirRepo.Get(ctx, dirRelPath)
	if err != nil {
		return nil, err
	}

	return r.factory.CreateFromInfo(ctx, info, absPath, dir.ID())
}

func (r *ImageRepository) Find(ctx context.Context, relPath string) iter.Seq2[*image.Image, error] {
	return func(yield func(*image.Image, error) bool) {
		absPath := filepath.Join(r.rootDir, relPath)
		entries, err := os.ReadDir(absPath)
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

				absFilePath := filepath.Join(absPath, entry.Name())
				info, err := entry.Info()
				if err != nil {
					return yield(nil, err)
				}

				img, err := r.factory.CreateFromInfo(ctx, info, absFilePath, directoryID)
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

func (r *ImageRepository) Lookup(ctx context.Context, relPath string) (*image.Image, error) {
	dirRelPath := filepath.Dir(relPath)
	if dirRelPath == "." {
		dirRelPath = ""
	}
	dir, err := r.dirRepo.Get(ctx, dirRelPath)
	if err != nil {
		return nil, err
	}
	return r.factory.Create(ctx, relPath, r.rootDir, dir.ID())
}

var _ image.Repository = (*ImageRepository)(nil)