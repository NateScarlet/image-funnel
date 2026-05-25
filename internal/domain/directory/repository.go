package directory

import (
	"context"
)

type Repository interface {
	Get(ctx context.Context, relPath string) (*Directory, error)
}
