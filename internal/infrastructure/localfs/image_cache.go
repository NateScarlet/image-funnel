package localfs

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	appimage "main/internal/application/image"
)

type ImageCache struct {
	rootDir         string
	cleanupInterval time.Duration
	maxAge          time.Duration
}

func NewImageCache(rootDir string, cleanupInterval, maxAge time.Duration) *ImageCache {
	os.MkdirAll(rootDir, 0755)
	return &ImageCache{
		rootDir:         rootDir,
		cleanupInterval: cleanupInterval,
		maxAge:          maxAge,
	}
}

func (c *ImageCache) GetPath(key string) string {
	return filepath.Join(c.rootDir, key)
}

func (c *ImageCache) Exists(key string) bool {
	path := c.GetPath(key)
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	// Update access/mod time to prevent cleanup
	now := time.Now()
	os.Chtimes(path, now, now)
	return !info.IsDir()
}

func (c *ImageCache) StartAutoClean(ctx context.Context) {
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
			if err := os.Remove(path); err != nil {
				log.Printf("Failed to remove old cache file %s: %v", path, err)
			}
		}
	}
}

var _ appimage.Cache = (*ImageCache)(nil)
