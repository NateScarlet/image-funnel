package inmem

import (
	"context"
	"iter"
	"main/internal/domain/directory"
	"main/internal/domain/image"
	"path/filepath"
	"sync"

	"go.uber.org/zap"
)

// DirectoryStatsCache 实现了 directory.Scanner 并缓存 AnalyzeDirectory 的结果
type DirectoryStatsCache struct {
	underlying directory.Scanner
	logger     *zap.Logger
	cache      sync.Map // key: relPath (string), value: *directory.DirectoryStats
}

// NewDirectoryStatsCache 创建目录统计缓存层
func NewDirectoryStatsCache(
	underlying directory.Scanner,
	logger *zap.Logger,
) *DirectoryStatsCache {
	return &DirectoryStatsCache{
		underlying: underlying,
		logger:     logger,
	}
}

// cacheKey 生成统一路径作为缓存 key
func (c *DirectoryStatsCache) cacheKey(relPath string) string {
	if relPath == "" {
		relPath = "."
	}
	return filepath.ToSlash(filepath.Clean(relPath))
}

// Invalidate clears the cache for a specific directory path.
func (c *DirectoryStatsCache) Invalidate(relPath string) {
	c.cache.Delete(c.cacheKey(relPath))
}

// AnalyzeDirectory 返回缓存的统计信息或委托给底层的 Scanner
func (c *DirectoryStatsCache) AnalyzeDirectory(ctx context.Context, relPath string) (*directory.DirectoryStats, error) {
	key := c.cacheKey(relPath)
	if val, ok := c.cache.Load(key); ok {
		return val.(*directory.DirectoryStats), nil
	}

	stats, err := c.underlying.AnalyzeDirectory(ctx, relPath)
	if err != nil {
		return nil, err
	}

	c.cache.Store(key, stats)
	return stats, nil
}

// Scan 委托给底层的 Scanner
func (c *DirectoryStatsCache) Scan(ctx context.Context, relPath string) iter.Seq2[*image.Image, error] {
	return c.underlying.Scan(ctx, relPath)
}

// LookupImage 委托给底层的 Scanner
func (c *DirectoryStatsCache) LookupImage(ctx context.Context, relPath string) (*image.Image, error) {
	return c.underlying.LookupImage(ctx, relPath)
}

// ScanDirectories 委托给底层的 Scanner
func (c *DirectoryStatsCache) ScanDirectories(ctx context.Context, relPath string) iter.Seq2[*directory.Directory, error] {
	return c.underlying.ScanDirectories(ctx, relPath)
}

var _ directory.Scanner = (*DirectoryStatsCache)(nil)
