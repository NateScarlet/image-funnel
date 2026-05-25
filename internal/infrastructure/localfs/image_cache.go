package localfs

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	appimage "main/internal/application/image"
	"main/internal/util"
)

type ImageCache struct {
	rootDir         string
	cleanupInterval time.Duration
	maxAge          time.Duration
}

func NewImageCache(rootDir string, cleanupInterval, maxAge time.Duration) (*ImageCache, func()) {
	cache := &ImageCache{
		rootDir:         rootDir,
		cleanupInterval: cleanupInterval,
		maxAge:          maxAge,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cache.startAutoClean(ctx)

	return cache, cancel
}

func (c *ImageCache) getPath(key string) string {
	return filepath.Join(c.rootDir, key)
}

func (c *ImageCache) Open(ctx context.Context, key string) (appimage.File, error) {
	path := c.getPath(key)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	if info.IsDir() {
		return nil, nil
	}

	now := time.Now()
	os.Chtimes(path, now, now)

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	return file, nil
}

func (c *ImageCache) Save(ctx context.Context, key string, r io.Reader) error {
	path := c.getPath(key)

	err := os.MkdirAll(filepath.Dir(path), 0755)
	if err != nil {
		return err
	}

	return util.AtomicSave(path, func(f *os.File) error {
		_, err := io.Copy(f, r)
		return err
	})
}

func (c *ImageCache) startAutoClean(ctx context.Context) {
	ticker := time.NewTicker(c.cleanupInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.cleanup()
			}
		}
	}()
}

func (c *ImageCache) cleanup() {
	entries, err := os.ReadDir(c.rootDir)
	if err != nil {
		log.Printf("Failed to read cache dir: %v", err)
		return
	}

	threshold := time.Now().Add(-c.maxAge)

	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(threshold) {
			path := filepath.Join(c.rootDir, entry.Name())
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				// 忽略文件已被其他进程或操作删除的情况，避免输出无意义的错误日志
				log.Printf("Failed to remove old cache file %s: %v", path, err)
			}
		}
	}
}

var _ appimage.Cache = (*ImageCache)(nil)
