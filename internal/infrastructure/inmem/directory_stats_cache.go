package inmem

import (
	"context"
	"main/internal/domain/directory"
	"path/filepath"
	"sync"

	"go.uber.org/zap"
)

// DirectoryStatsCache 实现了 directory.Analyzer 并缓存 Analyze 的结果
type DirectoryStatsCache struct {
	underlying directory.Analyzer
	logger     *zap.Logger
	cache      sync.Map // key: relPath (string), value: *directory.DirectoryStats
}

// NewDirectoryStatsCache 创建目录统计缓存层
func NewDirectoryStatsCache(
	underlying directory.Analyzer,
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

// Analyze 返回缓存的统计信息或委托给底层的 Analyzer
func (c *DirectoryStatsCache) Analyze(ctx context.Context, relPath string) (*directory.DirectoryStats, error) {
	key := c.cacheKey(relPath)
	if val, ok := c.cache.Load(key); ok {
		return val.(*directory.DirectoryStats), nil
	}

	stats, err := c.underlying.Analyze(ctx, relPath)
	if err != nil {
		return nil, err
	}

	c.cache.Store(key, stats)
	return stats, nil
}

var _ directory.Analyzer = (*DirectoryStatsCache)(nil)
