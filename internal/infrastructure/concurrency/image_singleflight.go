package concurrency

import (
	"context"
	"fmt"

	appimage "main/internal/application/image"

	"golang.org/x/sync/singleflight"
)

type SingleFlightImageProcessor struct {
	next  appimage.Processor
	group singleflight.Group
}

func NewSingleFlightImageProcessor(next appimage.Processor) *SingleFlightImageProcessor {
	return &SingleFlightImageProcessor{
		next: next,
	}
}

func (p *SingleFlightImageProcessor) Process(ctx context.Context, srcPath string, width, quality int) (string, error) {
	// Generate a key for request coalescing.
	// Note: This key depends only on input parameters.
	// If the underlying file changes, concurrent requests might receive the result of the first one.
	// This is an acceptable trade-off for cache stampede protection.
	key := fmt.Sprintf("%s|%d|%d", srcPath, width, quality)

	result, err, _ := p.group.Do(key, func() (interface{}, error) {
		return p.next.Process(context.Background(), srcPath, width, quality)
	})

	if err != nil {
		return "", err
	}

	return result.(string), nil
}

func (p *SingleFlightImageProcessor) Meta(ctx context.Context, srcPath string) (*appimage.ImageMeta, error) {
	return p.next.Meta(ctx, srcPath)
}

var _ appimage.Processor = (*SingleFlightImageProcessor)(nil)
