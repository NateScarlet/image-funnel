package image

import (
	"context"
	"io"

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

// Processor 纯转码端口：对单张源图执行一次转码并把结果流式交给调用方。
// 缓存与排队由应用层 TranscodeCoordinator 编排，实现方不负责
type Processor interface {
	Process(ctx context.Context, srcPath string, spec Spec, w io.Writer) error

	Meta(ctx context.Context, srcPath string) (*shared.ImageMeta, error)
}
