package http

import (
	"strconv"
	"strings"
	"time"
)

// durationMs 把 Go 时长换算为 Server-Timing 的毫秒数值
func durationMs(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000
}

// ServerTiming 一次图片响应的分段耗时，单位为毫秒。
// 队列与编码来自转码协调器，解码来自源图探针，命中状态来自变体缓存查询。
type ServerTiming struct {
	QueueMs  float64
	DecodeMs float64
	EncodeMs float64
	CacheHit bool
	Encoder  string
}

// FormatServerTiming 生成 Server-Timing 响应头。
// 指标名固定为 queue/decode/encode/cache；encode 用 desc 区分编码器，
// cache 用 desc 区分命中状态，便于读取侧无需猜测。
func FormatServerTiming(t ServerTiming) string {
	hit := "miss"
	if t.CacheHit {
		hit = "hit"
	}
	return strings.Join([]string{
		metricDuration("queue", t.QueueMs, ""),
		metricDuration("decode", t.DecodeMs, ""),
		metricDuration("encode", t.EncodeMs, quoteDesc(t.Encoder)),
		`cache;dur=0;desc="` + hit + `"`,
	}, ", ")
}

func metricDuration(name string, ms float64, desc string) string {
	value := "dur=" + strconv.FormatFloat(ms, 'f', 2, 64)
	if desc != "" {
		value += ";desc=" + desc
	}
	return name + ";" + value
}

// quoteDesc 按 Server-Timing 的 quoted-string 规则转义反斜杠与双引号
func quoteDesc(desc string) string {
	desc = strings.ReplaceAll(desc, `\`, `\\`)
	desc = strings.ReplaceAll(desc, `"`, `\"`)
	return `"` + desc + `"`
}
