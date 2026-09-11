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

// HybridProcessor 按格式分发转码的混合处理器：
// AVIF 交给 avif 编码器（ffmpeg/SVT-AV1，由 main 显式注入回退实现），其余交给
// fallback（magick）；元数据始终走 fallback（magick identify / Go 标准库解码）
type HybridProcessor struct {
	fallback appimage.Processor
	avif     appimage.Processor
}

// NewHybridProcessor 创建混合处理器。avif 是必传依赖（无 ffmpeg 时由装配方注入
// magick 作为显式回退实现，本组件不做可选依赖判断）
func NewHybridProcessor(fallback, avif appimage.Processor) *HybridProcessor {
	return &HybridProcessor{
		fallback: fallback,
		avif:     avif,
	}
}

func (p *HybridProcessor) Process(ctx context.Context, srcPath string, spec appimage.Spec, w io.Writer) error {
	if spec.Format() == appimage.ImageFormatAVIF {
		return p.avif.Process(ctx, srcPath, spec, w)
	}
	return p.fallback.Process(ctx, srcPath, spec, w)
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
