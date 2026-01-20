package image

import (
	"context"
)

type Processor interface {
	// Process returns the path to the processed image.
	// If width and quality are 0, it returns the original path.
	Process(ctx context.Context, srcPath string, width, quality int) (string, error)
}
