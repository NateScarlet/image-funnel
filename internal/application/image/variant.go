package image

import (
	"time"

	"main/internal/shared/transcode"
)

// VariantInput 取回图片变体的输入。一次调用即完成 key 计算、缓存查询、探源与响应形态决策；
// HEAD/GET 优先级通过 Priority 表达，避免调用方感知缓存与队列的两阶段知识。
type VariantInput struct {
	AbsPath  string
	Width    int                   // 0 表示全尺寸诉求
	Format   transcode.ImageFormat // 已协商的目标格式，缓存键与转码规格共用
	Accept   string                // 用于判定源图 MIME 是否可接受
	Priority transcode.Prio        // HEAD=低（预载登记），GET=高（当前查看）
}

// VariantResult 取回 seam 的输出，同时携带渲染 HTTP 响应所需的全部数据：
// 产物文件（服务缓存/转码分支）、重定向目标形态、转码格式与有效宽度，以及各段耗时。
type VariantResult struct {
	Kind           ResponseKind
	CanonicalWidth int
	Format         transcode.ImageFormat
	EffectiveWidth int
	File           transcode.File
	DecodeDuration time.Duration
	QueueDuration  time.Duration
	EncodeDuration time.Duration
	CacheHit       bool
}
