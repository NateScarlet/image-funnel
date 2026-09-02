package image

import (
	"context"

	"main/internal/shared"
)

// ImageFormat 图片转码输出格式
type ImageFormat int

const (
	ImageFormatWebP ImageFormat = iota
	ImageFormatAVIF
)

// String 返回格式的字符串表示
func (f ImageFormat) String() string {
	switch f {
	case ImageFormatAVIF:
		return "avif"
	default:
		return "webp"
	}
}

type Processor interface {
	Process(ctx context.Context, srcPath string, width, quality int, format ImageFormat) (File, error)

	Meta(ctx context.Context, srcPath string) (*shared.ImageMeta, error)
}
