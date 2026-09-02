package concurrency

import (
	"context"
	"fmt"

	appimage "main/internal/application/image"
	"main/internal/shared"

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

func (p *SingleFlightImageProcessor) Process(ctx context.Context, srcPath string, width, quality int, format appimage.ImageFormat) (appimage.File, error) {
	key := fmt.Sprintf("%s|%d|%d|%s", srcPath, width, quality, format.String())

	result, err, _ := p.group.Do(key, func() (interface{}, error) {
		return p.next.Process(context.Background(), srcPath, width, quality, format)
	})

	if err != nil {
		return nil, err
	}

	return result.(appimage.File), nil
}

func (p *SingleFlightImageProcessor) Meta(ctx context.Context, srcPath string) (*shared.ImageMeta, error) {
	return p.next.Meta(ctx, srcPath)
}

var _ appimage.Processor = (*SingleFlightImageProcessor)(nil)
