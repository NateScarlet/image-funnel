package http

import (
	"testing"

	"main/internal/shared/transcode"

	"github.com/stretchr/testify/assert"
)

func TestTargetFormat(t *testing.T) {
	// Accept 语义锁定：q 降序、同 q 固定 AVIF > WebP、specificity 覆盖、无明确要求默认 WebP
	tests := []struct {
		accept string
		want   transcode.ImageFormat
	}{
		{"", transcode.ImageFormatWebP},
		{"image/webp", transcode.ImageFormatWebP},
		{"image/avif", transcode.ImageFormatAVIF},
		{"image/avif, image/webp", transcode.ImageFormatAVIF},
		{"image/webp, image/avif", transcode.ImageFormatAVIF},
		{"*/*, image/webp", transcode.ImageFormatAVIF},
		{"image/webp, */*", transcode.ImageFormatAVIF},
		{"image/webp;q=0.9, image/avif;q=0.8", transcode.ImageFormatWebP},
		{"image/webp;q=0, */*;q=0.9", transcode.ImageFormatAVIF},
		{"image/jpeg", transcode.ImageFormatWebP},
	}
	for _, tt := range tests {
		t.Run(tt.accept, func(t *testing.T) {
			assert.Equal(t, tt.want, TargetFormat(tt.accept))
		})
	}
}

func TestDecideResponse(t *testing.T) {
	// 仅构造 ResponseInput 即可覆盖四类分支，不构造 endpoint handler 或其任何依赖。
	// CacheHit 为真/假都参与：命中时无条件 ServeCached，不命中时按其余字段决策。
	tests := []struct {
		name  string
		input ResponseInput
		want  ResponseDecision
	}{
		{
			name:  "缓存命中优先于全尺寸且源图可接受",
			input: ResponseInput{CacheHit: true, Width: 0, SourceWidth: 2048, SourceAcceptable: true, Format: transcode.ImageFormatWebP},
			want:  ResponseDecision{Kind: ResponseServeCached, Format: transcode.ImageFormatWebP},
		},
		{
			name:  "缓存命中优先于规范宽度重定向条件",
			input: ResponseInput{CacheHit: true, Width: 4096, SourceWidth: 512, SourceAcceptable: false, Format: transcode.ImageFormatAVIF},
			want:  ResponseDecision{Kind: ResponseServeCached, Format: transcode.ImageFormatAVIF},
		},
		{
			name:  "无宽度且源图可接受重定向到 raw",
			input: ResponseInput{Width: 0, SourceWidth: 2048, SourceAcceptable: true, Format: transcode.ImageFormatWebP},
			want:  ResponseDecision{Kind: ResponseRedirectRaw, Format: transcode.ImageFormatWebP},
		},
		{
			name:  "源宽不可知且全尺寸诉求重定向到 raw",
			input: ResponseInput{Width: 0, SourceWidth: 0, SourceAcceptable: true, Format: transcode.ImageFormatWebP},
			want:  ResponseDecision{Kind: ResponseRedirectRaw, Format: transcode.ImageFormatWebP},
		},
		{
			name:  "w 等于源宽且源图可接受重定向到 raw",
			input: ResponseInput{Width: 2048, SourceWidth: 2048, SourceAcceptable: true, Format: transcode.ImageFormatAVIF},
			want:  ResponseDecision{Kind: ResponseRedirectRaw, Format: transcode.ImageFormatAVIF},
		},
		{
			name:  "w 小于源宽且源图可接受仍转码",
			input: ResponseInput{Width: 1024, SourceWidth: 2048, SourceAcceptable: true, Format: transcode.ImageFormatWebP},
			want:  ResponseDecision{Kind: ResponseTranscode, Format: transcode.ImageFormatWebP, EffectiveWidth: 1024},
		},
		{
			name:  "源图不可接受且 w 超过源宽重定向到规范宽度",
			input: ResponseInput{Width: 4096, SourceWidth: 512, SourceAcceptable: false, Format: transcode.ImageFormatAVIF},
			want:  ResponseDecision{Kind: ResponseRedirectCanonicalWidth, Format: transcode.ImageFormatAVIF, CanonicalWidth: 512},
		},
		{
			name:  "源图不可接受且 w 等于源宽转码到源宽",
			input: ResponseInput{Width: 512, SourceWidth: 512, SourceAcceptable: false, Format: transcode.ImageFormatAVIF},
			want:  ResponseDecision{Kind: ResponseTranscode, Format: transcode.ImageFormatAVIF, EffectiveWidth: 512},
		},
		{
			name:  "源图不可接受且 w 小于源宽按 w 转码",
			input: ResponseInput{Width: 256, SourceWidth: 512, SourceAcceptable: false, Format: transcode.ImageFormatWebP},
			want:  ResponseDecision{Kind: ResponseTranscode, Format: transcode.ImageFormatWebP, EffectiveWidth: 256},
		},
		{
			name:  "源宽不可知时转码保持 w 不放大由编码器运行时保证",
			input: ResponseInput{Width: 4096, SourceWidth: 0, SourceAcceptable: false, Format: transcode.ImageFormatAVIF},
			want:  ResponseDecision{Kind: ResponseTranscode, Format: transcode.ImageFormatAVIF, EffectiveWidth: 4096},
		},
		{
			name:  "全尺寸诉求但源图不可接受时有效宽度为 0",
			input: ResponseInput{Width: 0, SourceWidth: 2048, SourceAcceptable: false, Format: transcode.ImageFormatWebP},
			want:  ResponseDecision{Kind: ResponseTranscode, Format: transcode.ImageFormatWebP, EffectiveWidth: 0},
		},
		{
			name:  "转码格式直接取输入不再二次协商",
			input: ResponseInput{Width: 1024, SourceWidth: 2048, SourceAcceptable: false, Format: transcode.ImageFormatAVIF},
			want:  ResponseDecision{Kind: ResponseTranscode, Format: transcode.ImageFormatAVIF, EffectiveWidth: 1024},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, DecideResponse(tt.input))
		})
	}
}
