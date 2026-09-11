package magick

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	appimage "main/internal/application/image"
	"main/internal/shared"

	"golang.org/x/sync/semaphore"
)

// Processor 纯转码执行器：不负责缓存（缓存由应用层 TranscodeCoordinator 编排），
// 只对单张源图执行一次 ImageMagick 转码并把结果流式交给调用方
type Processor struct {
	sem *semaphore.Weighted
}

func NewProcessor(concurrency int64) *Processor {
	if concurrency <= 0 {
		concurrency = 4
	}
	return &Processor{
		sem: semaphore.NewWeighted(concurrency),
	}
}

// Process 执行一次转码，结果流式写入 w。源文件状态（修改时间/大小）在执行时读取，
// 由调用方用于缓存键计算
func (p *Processor) Process(ctx context.Context, srcPath string, spec appimage.Spec, w io.Writer) error {
	// AVIF 全分辨率（无显式质量参数）使用质量上限 95
	quality := spec.Quality()
	if quality == 0 {
		quality = 95
	}

	if err := p.sem.Acquire(ctx, 1); err != nil {
		return err
	}
	defer p.sem.Release(1)

	args := []string{srcPath, "-coalesce"}
	if spec.Width() > 0 {
		args = append(args, "-resize", fmt.Sprintf("%dx>", spec.Width()))
	}
	args = append(args, "-quality", fmt.Sprintf("%d", quality))
	args = append(args, spec.Format().String()+":-")

	cmd := exec.CommandContext(ctx, "magick", args...)
	cmd.Stdout = w
	var b = new(bytes.Buffer)
	cmd.Stderr = b

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		errStr := b.String()
		// 识别文件尚未写完时的意外截止错误
		if strings.Contains(errStr, "unexpected end-of-file") || strings.Contains(errStr, "unexpected end of file") {
			return fmt.Errorf("%w: ImageMagick error: %s", io.ErrUnexpectedEOF, errStr)
		}
		return fmt.Errorf("ImageMagick error: %w, args: %v: stderr: %q", err, args, errStr)
	}
	return nil
}

func (p *Processor) Meta(ctx context.Context, absPath string) (*shared.ImageMeta, error) {
	if err := p.sem.Acquire(ctx, 1); err != nil {
		return nil, err
	}
	defer p.sem.Release(1)

	cmd := exec.CommandContext(ctx, "magick", "identify", "-ping", "-format", "%w %h", absPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		errStr := stderr.String()
		// 识别并转换为领域层标准错误
		if strings.Contains(errStr, "unexpected end-of-file") || strings.Contains(errStr, "unexpected end of file") {
			return nil, fmt.Errorf("%w: failed to get image metadata: %s", io.ErrUnexpectedEOF, errStr)
		}
		return nil, fmt.Errorf("failed to get image metadata: %w, stderr: %s", err, errStr)
	}

	var width, height int
	_, err = fmt.Sscanf(string(output), "%d %d", &width, &height)
	if err != nil {
		return nil, fmt.Errorf("failed to parse image dimensions: %w", err)
	}

	return &shared.ImageMeta{
		Width:  width,
		Height: height,
	}, nil
}

var _ appimage.Processor = (*Processor)(nil)
