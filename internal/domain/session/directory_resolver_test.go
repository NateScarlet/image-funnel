package session

import (
	"context"
	"main/internal/domain/directory"
	"main/internal/domain/relpath"
	"main/internal/scalar"
)

type FakeDirectoryResolver struct{}

func (r *FakeDirectoryResolver) GetDirectory(ctx context.Context, id scalar.ID) (*directory.Directory, error) {
	// 根目录是根相对路径的零值
	return directory.FromRepository(relpath.Path{}), nil
}
