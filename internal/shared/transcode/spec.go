package transcode

import "fmt"

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

// Spec 图片变体的转码规格：源图身份之外的变体参数。
// 只包含 width 与 format——画质由各编码器自身的配置决定（AVIF 见 svtav1CRF，
// WebP 见 magick 的固定质量），不作为变体维度，避免同一张图因画质参数不同重复编码
type Spec struct {
	width  int
	format ImageFormat
}

func NewSpec(width int, format ImageFormat) (Spec, error) {
	// 可信边界校验：宽度非负、格式合法；内部数据创建后即合法
	if width < 0 {
		return Spec{}, fmt.Errorf("width must be non-negative, got %d", width)
	}
	if format != ImageFormatWebP && format != ImageFormatAVIF {
		return Spec{}, fmt.Errorf("unsupported format: %d", format)
	}
	return Spec{width: width, format: format}, nil
}

func (s Spec) Width() int          { return s.width }
func (s Spec) Format() ImageFormat { return s.format }

// EffectiveWidth 是「不放大」规则的唯一实现：返回请求宽度在给定源宽下实际应产出的宽度。
//
// width == 0 表示全尺寸诉求；sourceWidth == 0 表示源宽不可知，此时没有可收缩的依据，
// 保持 width，不放大由编码器在运行时继续保证（magick 的 '>'、ffmpeg 的 min(iw,N)）。
// 两者均 > 0 时取较小值，使所有 w >= 源宽的请求收敛到同一份变体产物。
func EffectiveWidth(width, sourceWidth int) int {
	if width == 0 || sourceWidth == 0 {
		return width
	}
	return min(width, sourceWidth)
}
