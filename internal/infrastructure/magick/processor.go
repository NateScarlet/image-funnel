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
	"path/filepath"
	"strings"

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

type rawFile struct {
	path string
}

// Open 实现 appimage.File 接口，延迟打开原始图像文件。
func (f *rawFile) Open() (io.ReadSeekCloser, error) {
	return os.Open(f.path)
}

func (p *Processor) Process(ctx context.Context, absPath string, width, quality int, format appimage.ImageFormat) (appimage.File, error) {
	// WebP 且无缩放/质量参数时返回原始文件（向后兼容）；AVIF 全分辨率也需要转码
	if width == 0 && quality == 0 && format == appimage.ImageFormatWebP {
		return &rawFile{path: absPath}, nil
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}

	// AVIF 全分辨率（无显式质量参数）使用质量上限 95
	effectiveQuality := quality
	if effectiveQuality == 0 {
		effectiveQuality = 95
	}

	timestamp := fmt.Sprintf("%d", info.ModTime().UnixNano())
	size := fmt.Sprintf("%d", info.Size())
	wStr := ""
	if width > 0 {
		wStr = fmt.Sprintf("%d", width)
	}
	qStr := ""
	if effectiveQuality > 0 {
		qStr = fmt.Sprintf("%d", effectiveQuality)
	}
	formatStr := format.String()

	hash := sha256.New()
	// 使用文件名而非绝对路径，从而允许文件移动后复用已转码的缓存（只要文件名、修改时间和大小不变）
	fmt.Fprintf(hash, "%s|%s|%s|%s|%s|%s", filepath.Base(absPath), timestamp, size, wStr, qStr, formatStr)

	cacheKey := base64.URLEncoding.EncodeToString(hash.Sum(nil))

	file, err := p.cache.Lookup(ctx, cacheKey)
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

		args := []string{absPath, "-coalesce"}
		if width > 0 {
			args = append(args, "-resize", fmt.Sprintf("%dx>", width))
		}
		if effectiveQuality > 0 {
			args = append(args, "-quality", fmt.Sprintf("%d", effectiveQuality))
		}
		args = append(args, format.String()+":-")

		cmd := exec.CommandContext(ctx, "magick", args...)
		cmd.Stdout = pipeWriter
		var b = new(bytes.Buffer)
		cmd.Stderr = b

		if err := cmd.Run(); err != nil {
			if ctx.Err() != nil {
				pipeWriter.CloseWithError(ctx.Err())
				return
			}
			errStr := b.String()
			// 识别文件尚未写完时的意外截止错误
			if strings.Contains(errStr, "unexpected end-of-file") || strings.Contains(errStr, "unexpected end of file") {
				pipeWriter.CloseWithError(fmt.Errorf("%w: ImageMagick error: %s", io.ErrUnexpectedEOF, errStr))
				return
			}
			pipeWriter.CloseWithError(fmt.Errorf("ImageMagick error: %w, args: %v: stderr: %q", err, args, errStr))
			return
		}
	}()

	saveErr := p.cache.Save(ctx, cacheKey, pipeReader)
	if saveErr != nil {
		return nil, saveErr
	}

	return p.cache.Lookup(ctx, cacheKey)
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
