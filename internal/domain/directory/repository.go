package directory

import (
	"context"
	"main/internal/scalar"
)

type Repository interface {
	Get(ctx context.Context, id scalar.ID) (*Directory, error)
	GetByRelPath(ctx context.Context, relPath string) (*Directory, error)
}
