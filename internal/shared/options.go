package shared

import "main/internal/scalar"

// MarkImageOptions 包含标记图片时的可选参数
type MarkImageOptions struct {
	duration scalar.Duration
}

// MarkImageOption 是用于设置 MarkImageOptions 的函数类型
type MarkImageOption func(*MarkImageOptions)

// NewMarkImageOptions 创建一个新的 MarkImageOptions 实例
func NewMarkImageOptions(opts ...MarkImageOption) *MarkImageOptions {
	o := &MarkImageOptions{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// WithDuration 设置操作耗时
func WithDuration(d scalar.Duration) MarkImageOption {
	return func(o *MarkImageOptions) {
		o.duration = d
	}
}

// Duration 获取操作耗时
func (o *MarkImageOptions) Duration() scalar.Duration {
	return o.duration
}
