package samplearchive

import (
	"fmt"
	stdimage "image"
	"os"
	"sync"

	_ "image/jpeg"
	_ "image/png"

	"main/internal/domain/session"
	"main/internal/scalar"

	_ "github.com/chai2010/webp"
)

// CommitImageDecoder 在单次 Commit 内按版本化图像 ID 复用解码后的图像。
// 大图解码是采样的主要开销之一，而同一基准图常被反复比较与编码；
// 缓存生命周期跟随 Commit，结束即丢弃，不跨 Commit 共享。
type CommitImageDecoder struct {
	baseDir string
	mu      sync.Mutex
	images  map[scalar.ID]stdimage.Image
}

var _ session.ImageDecoder = (*CommitImageDecoder)(nil)

// NewCommitImageDecoder 创建一次 Commit 专用的解码器；baseDir 用于解析相对路径。
func NewCommitImageDecoder(baseDir string) *CommitImageDecoder {
	return &CommitImageDecoder{
		baseDir: baseDir,
		images:  make(map[scalar.ID]stdimage.Image),
	}
}

// Decode 返回已解码的图像：命中缓存不读盘，未命中则解码后以图像 ID 为键写入缓存。
func (d *CommitImageDecoder) Decode(image session.SampleImage) (stdimage.Image, error) {
	d.mu.Lock()
	img, cached := d.images[image.ID]
	d.mu.Unlock()
	if cached {
		return img, nil
	}

	absPath := image.RelPath.Abs(d.baseDir)
	file, err := os.Open(absPath)
	if err != nil {
		return nil, fmt.Errorf("open source image %s: %w", absPath, err)
	}
	defer file.Close()

	img, _, err = stdimage.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("decode source image %s: %w", absPath, err)
	}
	d.mu.Lock()
	d.images[image.ID] = img
	d.mu.Unlock()
	return img, nil
}
