package image

import (
	"context"
	"errors"
	"io"
	"time"

	"main/internal/shared"

	"go.uber.org/zap"
)

// RetryProcessor 是一个用于在检测到 unexpected EOF 错误时进行指数退避重试的处理器装饰器
type RetryProcessor struct {
	next        Processor
	logger      *zap.Logger
	maxAttempts int
	backoff     time.Duration
}

// NewRetryProcessor 创建一个新的重试处理器装饰器实例
func NewRetryProcessor(next Processor, logger *zap.Logger) *RetryProcessor {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &RetryProcessor{
		next:        next,
		logger:      logger,
		maxAttempts: 5,
		backoff:     200 * time.Millisecond,
	}
}

// Process 尝试对图片进行转码，如果在处理中发生意外截止错误，在 context 允许的范围内进行指数退避重试
func (p *RetryProcessor) Process(ctx context.Context, srcPath string, width, quality int) (File, error) {
	return retry(ctx, p, srcPath, "process", func() (File, error) {
		return p.next.Process(ctx, srcPath, width, quality)
	})
}

// Meta 尝试获取图片元数据，如果因文件未写完出现意外截止错误，在后台进行指数退避重试
func (p *RetryProcessor) Meta(ctx context.Context, srcPath string) (*shared.ImageMeta, error) {
	return retry(ctx, p, srcPath, "metadata", func() (*shared.ImageMeta, error) {
		return p.next.Meta(ctx, srcPath)
	})
}

// retry 泛型重试辅助函数，仅对 io.ErrUnexpectedEOF 错误执行指数退避重试
func retry[T any](ctx context.Context, p *RetryProcessor, srcPath, opName string, fn func() (T, error)) (T, error) {
	var result T
	var err error

	for attempt := 1; attempt <= p.maxAttempts; attempt++ {
		result, err = fn()
		if err == nil {
			return result, nil
		}

		// 仅对 unexpected EOF 进行重试
		if !errors.Is(err, io.ErrUnexpectedEOF) || ctx.Err() != nil {
			return result, err
		}

		p.logger.Warn("unexpected end of file, retrying with exponential backoff",
			zap.String("op", opName),
			zap.String("path", srcPath),
			zap.Int("attempt", attempt),
			zap.Error(err),
		)

		// 指数退避时间计算：backoff * 2^(attempt - 1)
		// 1<<uint(attempt-1) 计算 2 的幂次方
		backoffDuration := p.backoff * time.Duration(1<<uint(attempt-1))

		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(backoffDuration):
		}
	}

	return result, err
}

var _ Processor = (*RetryProcessor)(nil)
