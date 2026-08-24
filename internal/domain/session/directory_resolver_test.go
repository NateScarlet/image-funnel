package session

import (
	"context"
	"main/internal/domain/directory"
	"main/internal/scalar"
)

type FakeDirectoryResolver struct{}

func (r *FakeDirectoryResolver) GetDirectory(ctx context.Context, id scalar.ID) (*directory.Directory, error) {
	return directory.FromRepository("."), nil
}
