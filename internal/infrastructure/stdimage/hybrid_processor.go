package stdimage

import (
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	appimage "main/internal/application/image"
	"main/internal/shared"

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

func (p *HybridProcessor) Process(ctx context.Context, srcPath string, width, quality int, format appimage.ImageFormat) (appimage.File, error) {
	return p.fallback.Process(ctx, srcPath, width, quality, format)
}

func (p *HybridProcessor) Meta(ctx context.Context, srcPath string) (*shared.ImageMeta, error) {
	ext := strings.ToLower(filepath.Ext(srcPath))

	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		return p.getImageMeta(srcPath)
	default:
		return p.fallback.Meta(ctx, srcPath)
	}
}

func (p *HybridProcessor) getImageMeta(srcPath string) (*shared.ImageMeta, error) {
	file, err := os.Open(srcPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open image: %w", err)
	}
	defer file.Close()

	config, _, err := image.DecodeConfig(file)
	if err != nil {
		// 检查是否为意外截断的 unexpected EOF 错误
		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) ||
			strings.Contains(err.Error(), "unexpected EOF") ||
			strings.Contains(err.Error(), "unexpected end-of-file") {
			return nil, fmt.Errorf("%w: failed to decode image config: %v", io.ErrUnexpectedEOF, err)
		}
		return nil, fmt.Errorf("failed to decode image config: %w", err)
	}

	return &shared.ImageMeta{
		Width:  config.Width,
		Height: config.Height,
	}, nil
}

var _ appimage.Processor = (*HybridProcessor)(nil)
