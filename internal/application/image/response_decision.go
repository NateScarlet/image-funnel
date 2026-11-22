package image

import (
	"main/internal/shared/transcode"
	"main/internal/util"
)

// ResponseKind /image 的响应形态
type ResponseKind int

const (
	ResponseServeCached            ResponseKind = iota // 服务已存在的变体缓存产物
	ResponseRedirectRaw                                // 302 到 raw URL
	ResponseRedirectCanonicalWidth                     // 302 到 w=源宽 的规范 URL
	ResponseTranscode                                  // 转码到指定格式与有效宽度
)

// ResponseInput 响应形态决策的输入。CacheHit 是显式字段，缓存命中/未命中由它直接表达；
// Format 由调用方用 TargetFormat 算出后传入，使一次请求只协商一次格式。
type ResponseInput struct {
	Accept           string
	Width            int // 0 表示全尺寸诉求
	SourceWidth      int // 0 表示源宽不可知
	SourceAcceptable bool
	CacheHit         bool
	Format           transcode.ImageFormat
}

// ResponseDecision 一次决策产出的完整响应形态，只含对应分支需要的数据
type ResponseDecision struct {
	Kind           ResponseKind
	CanonicalWidth int // RedirectCanonicalWidth：规范 URL 的宽度
	Format         transcode.ImageFormat
	EffectiveWidth int // Transcode：不放大后的有效宽度
}

// TargetFormat 根据 Accept 返回用于缓存键与转码的目标格式。
// 优先级语义由 util 层锁定：q 降序、同 q 固定 AVIF > WebP（ADR 0010），无明确要求默认 WebP。
func TargetFormat(accept string) transcode.ImageFormat {
	for _, pf := range util.PreferredImageFormats(accept) {
		switch pf {
		case "image/avif":
			return transcode.ImageFormatAVIF
		case "image/webp":
			return transcode.ImageFormatWebP
		}
	}
	return transcode.ImageFormatWebP
}

// DecideResponse 一次调用产出完整响应形态，不依赖签名器、协调器、探针或 HTTP 请求对象。
//
// 分支规则保持现有语义：缓存命中即 ServeCached；全尺寸且源图 MIME 可接受即 RedirectRaw；
// 源图 MIME 不可接受且 Width > SourceWidth > 0 即 RedirectCanonicalWidth；其余为 Transcode。
func DecideResponse(input ResponseInput) ResponseDecision {
	if input.CacheHit {
		return ResponseDecision{Kind: ResponseServeCached, Format: input.Format}
	}

	fullSize := input.Width == 0 || (input.SourceWidth > 0 && input.Width >= input.SourceWidth)
	if fullSize && input.SourceAcceptable {
		return ResponseDecision{Kind: ResponseRedirectRaw, Format: input.Format}
	}

	// 源图 MIME 不被接受时，全尺寸产物与 w=源宽 的转码产物字节相同；
	// 让 w 严格大于源宽的请求 302 到该规范 URL，客户端便只占一个缓存条目。
	// 严格大于（而非 >=）避免规范 URL 自重定向；源宽不可知（0）不重定向
	if !input.SourceAcceptable && input.SourceWidth > 0 && input.Width > input.SourceWidth {
		return ResponseDecision{
			Kind:           ResponseRedirectCanonicalWidth,
			Format:         input.Format,
			CanonicalWidth: input.SourceWidth,
		}
	}

	return ResponseDecision{
		Kind:           ResponseTranscode,
		Format:         input.Format,
		EffectiveWidth: transcode.EffectiveWidth(input.Width, input.SourceWidth),
	}
}
