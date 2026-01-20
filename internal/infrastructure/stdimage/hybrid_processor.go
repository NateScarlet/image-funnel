package stdimage

import (
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	appimage "main/internal/application/image"

	_ "golang.org/x/image/webp"
)

type HybridProcessor struct {
	fallback appimage.Processor
}

func NewHybridProcessor(fallback appimage.Processor) *HybridProcessor {
	return &HybridProcessor{
		fallback: fallback,
	}
}

func (p *HybridProcessor) Process(ctx context.Context, srcPath string, width, quality int) (string, error) {
	return p.fallback.Process(ctx, srcPath, width, quality)
}

func (p *HybridProcessor) Meta(ctx context.Context, srcPath string) (*appimage.ImageMeta, error) {
	ext := strings.ToLower(filepath.Ext(srcPath))

	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		return p.getImageMeta(srcPath)
	default:
		return p.fallback.Meta(ctx, srcPath)
	}
}

func (p *HybridProcessor) getImageMeta(srcPath string) (*appimage.ImageMeta, error) {
	file, err := os.Open(srcPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open image: %w", err)
	}
	defer file.Close()

	config, _, err := image.DecodeConfig(file)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image config: %w", err)
	}

	return &appimage.ImageMeta{
		Width:  config.Width,
		Height: config.Height,
	}, nil
}

var _ appimage.Processor = (*HybridProcessor)(nil)
