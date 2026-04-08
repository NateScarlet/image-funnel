package image

import (
	"context"

	"main/internal/shared"
)

type Processor interface {
	Process(ctx context.Context, srcPath string, width, quality int) (File, error)

	Meta(ctx context.Context, srcPath string) (*shared.ImageMeta, error)
}
