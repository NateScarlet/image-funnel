package localfs

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"time"

	appimage "main/internal/application/image"
	"main/internal/util"

	"go.uber.org/zap"
)

type ImageCache struct {
	rootDir         string
	cleanupInterval time.Duration
	maxAge          time.Duration
	logger          *zap.Logger
}

func NewImageCache(rootDir string, cleanupInterval, maxAge time.Duration, logger *zap.Logger) (*ImageCache, func()) {
	cache := &ImageCache{
		rootDir:         rootDir,
		cleanupInterval: cleanupInterval,
		maxAge:          maxAge,
		logger:          logger,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cache.startAutoClean(ctx)

	return cache, cancel
}

func (c *ImageCache) getPath(key string) string {
	return filepath.Join(c.rootDir, key)
}

type cachedFile struct {
	path string
}

// Open 实现 appimage.File 接口，返回一个独立的文件读取器。
func (f *cachedFile) Open() (io.ReadSeekCloser, error) {
	now := time.Now()
	// 每次打开更新文件的访问与修改时间，以防其被缓存清理逻辑判定为过期
	os.Chtimes(f.path, now, now)

	file, err := os.Open(f.path)
	if err != nil {
		return nil, err
	}
	return file, nil
}

// Lookup 根据 key 获取文件句柄。如果文件不存在，返回 (nil, nil)。
func (c *ImageCache) Lookup(ctx context.Context, key string) (appimage.File, error) {
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

	return &cachedFile{path: path}, nil
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
		c.logger.Error("failed to read cache dir", zap.Error(err))
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
				c.logger.Error("failed to remove old cache file", zap.String("path", path), zap.Error(err))
			}
		}
	}
}

var _ appimage.Cache = (*ImageCache)(nil)
