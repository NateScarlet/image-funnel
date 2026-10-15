package inmem

import (
	"context"
	"main/internal/domain/directory"
	"main/internal/domain/relpath"
	"sync"

	"go.uber.org/zap"
)

// DirectoryStatsCache 实现了 directory.Analyzer 并缓存 Analyze 的结果
type DirectoryStatsCache struct {
	underlying directory.Analyzer
	logger     *zap.Logger
	// key 直接使用根相对路径：它是可比较的值，根目录的零值天然与普通路径区分开
	cache sync.Map
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

// Invalidate clears the cache for a specific directory path.
func (c *DirectoryStatsCache) Invalidate(relPath relpath.Path) {
	c.cache.Delete(relPath)
}

// Analyze 返回缓存的统计信息或委托给底层的 Analyzer
func (c *DirectoryStatsCache) Analyze(ctx context.Context, relPath relpath.Path) (*directory.Stats, error) {
	if val, ok := c.cache.Load(relPath); ok {
		return val.(*directory.Stats), nil
	}

	stats, err := c.underlying.Analyze(ctx, relPath)
	if err != nil {
		return nil, err
	}

	c.cache.Store(relPath, stats)
	return stats, nil
}

var _ directory.Analyzer = (*DirectoryStatsCache)(nil)
