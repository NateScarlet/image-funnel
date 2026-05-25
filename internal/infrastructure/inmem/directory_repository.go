package inmem

import (
	"context"
	"main/internal/domain/directory"
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
func (d *DirectoryRepository) Get(ctx context.Context, relPath string) (*directory.Directory, error) {
	err := util.EnsurePathInRoot(d.rootDir, relPath)
	if err != nil {
		return nil, err
	}
	return directory.FromRepository(relPath), nil
}

var _ directory.Repository = (*DirectoryRepository)(nil)
