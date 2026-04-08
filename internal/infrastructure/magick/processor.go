package magick

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"

	appimage "main/internal/application/image"
	"main/internal/shared"

	"golang.org/x/sync/semaphore"
)

type Processor struct {
	cache appimage.Cache
	sem   *semaphore.Weighted
}

func NewProcessor(cache appimage.Cache, concurrency int64) *Processor {
	if concurrency <= 0 {
		concurrency = 4
	}
	return &Processor{
		cache: cache,
		sem:   semaphore.NewWeighted(concurrency),
	}
}

func (p *Processor) Process(ctx context.Context, srcPath string, width, quality int) (appimage.File, error) {
	if width == 0 && quality == 0 {
		return os.Open(srcPath)
	}

	info, err := os.Stat(srcPath)
	if err != nil {
		return nil, err
	}

	timestamp := fmt.Sprintf("%d", info.ModTime().Unix())
	size := fmt.Sprintf("%d", info.Size())
	wStr := ""
	if width > 0 {
		wStr = fmt.Sprintf("%d", width)
	}
	qStr := ""
	if quality > 0 {
		qStr = fmt.Sprintf("%d", quality)
	}

	hash := sha256.New()
	fmt.Fprintf(hash, "%s|%s|%s|%s|%s", srcPath, timestamp, size, wStr, qStr)

	cacheKey := base64.URLEncoding.EncodeToString(hash.Sum(nil))

	file, err := p.cache.Open(ctx, cacheKey)
	if err != nil {
		return nil, err
	}
	if file != nil {
		return file, nil
	}

	pipeReader, pipeWriter := io.Pipe()

	go func() {
		defer pipeWriter.Close()

		if err := p.sem.Acquire(ctx, 1); err != nil {
			pipeWriter.CloseWithError(err)
			return
		}
		defer p.sem.Release(1)

		args := []string{srcPath, "-coalesce"}
		if width > 0 {
			args = append(args, "-resize", fmt.Sprintf("%dx>", width))
		}
		if quality > 0 {
			args = append(args, "-quality", fmt.Sprintf("%d", quality))
		}
		args = append(args, "webp:-")

		cmd := exec.CommandContext(ctx, "magick", args...)
		cmd.Stdout = pipeWriter
		var b = new(bytes.Buffer)
		cmd.Stderr = b

		if err := cmd.Run(); err != nil {
			if ctx.Err() != nil {
				pipeWriter.CloseWithError(ctx.Err())
				return
			}
			pipeWriter.CloseWithError(fmt.Errorf("ImageMagick error: %w, args: %v: stderr: %q", err, args, b.String()))
			return
		}
	}()

	saveErr := p.cache.Save(ctx, cacheKey, pipeReader)
	if saveErr != nil {
		return nil, saveErr
	}

	return p.cache.Open(ctx, cacheKey)
}

func (p *Processor) Meta(ctx context.Context, srcPath string) (*shared.ImageMeta, error) {
	if err := p.sem.Acquire(ctx, 1); err != nil {
		return nil, err
	}
	defer p.sem.Release(1)

	cmd := exec.CommandContext(ctx, "magick", "identify", "-ping", "-format", "%w %h", srcPath)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get image metadata: %w", err)
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
