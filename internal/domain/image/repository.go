package image

import (
	"context"
)

type Repository interface {
	Get(ctx context.Context, absPath string) (*Image, error)
}