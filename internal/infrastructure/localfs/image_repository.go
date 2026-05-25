package localfs

import (
	"context"
	"main/internal/domain/directory"
	"main/internal/domain/image"
	"os"
	"path/filepath"
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

var _ image.Repository = (*ImageRepository)(nil)