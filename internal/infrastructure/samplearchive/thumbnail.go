package samplearchive

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"

	"golang.org/x/sync/semaphore"
)

const (
	thumbnailSize    = 224
	thumbnailQuality = 75
)

// MagickThumbnailer 用 ImageMagick 在提交当下生成固定 224×224 的居中裁剪 WebP 缩略图。
// 缩略图写入独立持久目录，不依赖会过期的变体缓存。
type MagickThumbnailer struct {
	sem *semaphore.Weighted
}

func NewMagickThumbnailer(concurrency int64) *MagickThumbnailer {
	if concurrency <= 0 {
		concurrency = 4
	}
	return &MagickThumbnailer{sem: semaphore.NewWeighted(concurrency)}
}

func (m *MagickThumbnailer) Thumbnail(ctx context.Context, srcPath string) ([]byte, error) {
	if err := m.sem.Acquire(ctx, 1); err != nil {
		return nil, err
	}
	defer m.sem.Release(1)

	size := fmt.Sprintf("%dx%d", thumbnailSize, thumbnailSize)
	args := []string{
		srcPath,
		"-coalesce",
		"-resize", size + "^",
		"-gravity", "center",
		"-extent", size,
		"-quality", fmt.Sprint(thumbnailQuality),
		"webp:-",
	}
	cmd := exec.CommandContext(ctx, "magick", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("ImageMagick thumbnail error: %w, stderr: %q", err, stderr.String())
	}
	return stdout.Bytes(), nil
}

var _ Thumbnailer = (*MagickThumbnailer)(nil)
