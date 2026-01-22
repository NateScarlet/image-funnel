package directory

import (
	"context"
	"main/internal/scalar"
)

type Repository interface {
	Get(ctx context.Context, id scalar.ID) (*Directory, error)
	GetByPath(ctx context.Context, path string) (*Directory, error)
}
