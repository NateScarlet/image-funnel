package magick

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"

	appimage "main/internal/application/image"
	"main/internal/shared"
	"main/internal/util"
)

type Processor struct {
	cache appimage.Cache
}

func NewProcessor(cache appimage.Cache) *Processor {
	return &Processor{
		cache: cache,
	}
}

func (p *Processor) Process(ctx context.Context, srcPath string, width, quality int) (string, error) {
	info, err := os.Stat(srcPath)
	if err != nil {
		return "", err
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

	if p.cache.Exists(cacheKey) {
		return p.cache.GetPath(cacheKey), nil
	}

	cachePath := p.cache.GetPath(cacheKey)

	// Use AtomicSave to write the file securely
	err = util.AtomicSave(cachePath, func(f *os.File) error {
		args := []string{srcPath}
		if width > 0 {
			args = append(args, "-resize", fmt.Sprintf("%dx>", width))
		}
		if quality > 0 {
			args = append(args, "-quality", fmt.Sprintf("%d", quality))
		}
		args = append(args, "webp:-")

		cmd := exec.CommandContext(ctx, "magick", args...)
		cmd.Stdout = f // Write directly to the temp file
		// Capture stderr for debugging
		var b = new(bytes.Buffer)
		cmd.Stderr = b

		if err := cmd.Run(); err != nil {
			// If the context was canceled, return that error specifically so the caller knows it wasn't a process failure.
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("ImageMagick error: %w, args: %v: stderr: %q", err, args, b.String())
		}
		return nil
	})

	return cachePath, err
}

func (p *Processor) Meta(ctx context.Context, srcPath string) (*shared.ImageMeta, error) {
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
