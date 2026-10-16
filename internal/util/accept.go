package util

import (
	"sort"
	"strconv"
	"strings"
)

// #region Accept 头解析与有效 q 计算

// mediaRange 是 Accept 头中的一条 media-range（如 "image/webp;q=0.8"）
type mediaRange struct {
	Type    string // 如 "image/avif"、"image/*"、"*/*"
	Quality float64
}

// parseAccept 解析 Accept 头为 media-range 列表，保持书写顺序。
//
// 顺序不承载语义（RFC 9110 §12.5.1：同 q 即同等偏好，见 ADR 0010），
// 因此这里不排序，取舍分别由 Accepts（specificity 覆盖）与
// PreferredImageFormats（q 降序 + 同 q 固定优先级）决定。
func parseAccept(accept string) []mediaRange {
	var ranges []mediaRange

	for _, part := range strings.Split(accept, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// 首段是类型，其后是参数（其中只有 q 参与协商）
		segments := strings.Split(part, ";")
		mediaType := strings.TrimSpace(segments[0])
		if mediaType == "" {
			continue
		}

		r := mediaRange{Type: mediaType, Quality: 1} // q 缺省为 1
		for _, param := range segments[1:] {
			key, value, ok := strings.Cut(param, "=")
			if !ok || strings.TrimSpace(key) != "q" {
				continue
			}
			// 无法解析的 q 视为未提供（保持缺省值）：Accept 是客户端自由填写的字段，
			// 不因单个参数写法不合法而整体失败
			if q, err := strconv.ParseFloat(strings.Trim(strings.TrimSpace(value), `"`), 64); err == nil {
				r.Quality = q
			}
		}

		ranges = append(ranges, r)
	}

	return ranges
}

// matchRange 判断 media-range 是否覆盖具体类型 contentType，覆盖时返回其匹配精度：
// */* = 0，type/* = 1，type/subtype = 2
func matchRange(rangeType, contentType string) (specificity int, covers bool) {
	switch {
	case rangeType == "*/*":
		return 0, true
	case strings.HasSuffix(rangeType, "/*"):
		return 1, strings.HasPrefix(contentType, strings.TrimSuffix(rangeType, "*"))
	default:
		return 2, rangeType == contentType
	}
}

// effectiveQuality 求 contentType 在 Accept 头下的有效 q 值，无任何 media-range 覆盖时为 0。
//
// RFC 9110 §12.5.1：一个类型的 q 值由覆盖它的最 specific 的 media-range 决定，
// 因此 "image/jpeg;q=0, */*" 中 jpeg 的有效 q 是 0 而非通配符的 q。
func effectiveQuality(ranges []mediaRange, contentType string) float64 {
	quality := 0.0
	bestSpecificity := -1

	for _, r := range ranges {
		s, covers := matchRange(r.Type, contentType)
		if !covers {
			continue
		}
		switch {
		case s > bestSpecificity:
			bestSpecificity, quality = s, r.Quality
		case s == bestSpecificity && r.Quality > quality:
			// 同一类型被重复书写时 q 值无定义，取其中最大者，避免随机判定为拒绝
			quality = r.Quality
		}
	}

	return quality
}

// #endregion

// Accepts 判断 contentType 是否被 Accept 头接受。
//
// 该判定服务于任意 MIME 类型（如源图能否直出），不限于受支持的输出格式。
// 没有任何 media-range 覆盖 contentType 时不可接受，Accept 头缺失同样落入此分支。
func Accepts(accept, contentType string) bool {
	return effectiveQuality(parseAccept(accept), contentType) > 0
}

// supportedImageFormats 受支持的转码目标，列表顺序即同 q 时的固定优先级（ADR 0010：AVIF > WebP）
var supportedImageFormats = []string{"image/avif", "image/webp"}

// PreferredImageFormats 返回客户端接受的受支持转码格式，按优先级降序。
//
// 每个受支持格式按其有效 q（覆盖它的最 specific 的 media-range 决定）入选：
// 有效 q>0 即入选，因此被显式 q=0 拒绝的格式不会因通配符而回归。
// 有效 q 相同（含仅由通配符覆盖）时按 supportedImageFormats 的顺序取舍，即 AVIF > WebP。
//
// 返回值只含具体格式：通配符（image/*、*/*）表达的是「可接受」，
// 它通过有效 q 让具体格式入选，自身不被当作转码目标。
// 无格式可接受时返回 nil，由调用者决定兜底格式。
func PreferredImageFormats(accept string) []string {
	ranges := parseAccept(accept)

	type candidate struct {
		format  string
		quality float64
	}
	var candidates []candidate
	for _, format := range supportedImageFormats {
		if q := effectiveQuality(ranges, format); q > 0 {
			candidates = append(candidates, candidate{format: format, quality: q})
		}
	}

	// 稳定排序：同 q 时保持 supportedImageFormats 的固定优先级
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].quality > candidates[j].quality
	})

	var formats []string
	for _, c := range candidates {
		formats = append(formats, c.format)
	}
	return formats
}
