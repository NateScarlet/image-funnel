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

func (r *ImageRepository) Get(ctx context.Context, relPath string) (*image.Image, error) {
	// 校验以确保路径不是绝对路径，防御性拒绝绝对路径的入参
	if filepath.IsAbs(relPath) {
		return nil, fmt.Errorf("absolute path not allowed: %s", relPath)
	}

	absPath := filepath.Join(r.rootDir, relPath)
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}

	dirRelPath := filepath.Dir(relPath)
	dir, err := r.dirRepo.Get(ctx, dirRelPath)
	if err != nil {
		return nil, err
	}

	return r.factory.CreateFromInfo(ctx, info, relPath, dir.ID())
}

func (r *ImageRepository) Find(ctx context.Context, relPath string) iter.Seq2[*image.Image, error] {
	// 校验以确保路径不是绝对路径，防御性拒绝绝对路径的入参
	if filepath.IsAbs(relPath) {
		return func(yield func(*image.Image, error) bool) {
			yield(nil, fmt.Errorf("absolute path not allowed: %s", relPath))
		}
	}

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

				fileRelPath := filepath.Join(relPath, entry.Name())
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