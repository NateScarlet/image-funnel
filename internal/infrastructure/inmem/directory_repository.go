package inmem

import (
	"context"
	"main/internal/domain/directory"
	"main/internal/scalar"
	"main/internal/util"
)

func NewDirectoryRepository(rootDir string) *DirectoryRepository {
	return &DirectoryRepository{
		rootDir: rootDir,
	}
}

type DirectoryRepository struct {
	rootDir string
}

// Get implements [directory.Repository].
func (d *DirectoryRepository) Get(ctx context.Context, id scalar.ID) (*directory.Directory, error) {
	path, err := directory.DecodeID(id)
	if err != nil {
		return nil, err
	}
	err = util.EnsurePathInRoot(d.rootDir, path)
	if err != nil {
		return nil, err
	}
	return directory.FromRepository(id, path), nil
}

// GetByPath implements [directory.Repository].
func (d *DirectoryRepository) GetByPath(ctx context.Context, path string) (*directory.Directory, error) {
	var err = util.EnsurePathInRoot(d.rootDir, path)
	if err != nil {
		return nil, err
	}
	id := directory.EncodeID(path)
	return directory.FromRepository(id, path), nil
}

var _ directory.Repository = (*DirectoryRepository)(nil)
