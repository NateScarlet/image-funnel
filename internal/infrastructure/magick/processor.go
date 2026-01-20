package magick

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"os/exec"

	appimage "main/internal/application/image"
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
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			log.Printf("ImageMagick error: %v, args: %v", err, args)
			return err
		}
		return nil
	})

	return cachePath, err
}

var _ appimage.Processor = (*Processor)(nil)
