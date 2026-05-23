package directory

import (
	"context"
)

// Analyzer 负责深入分析目录内部结构以提供聚合数据
type Analyzer interface {
	// Analyze 计算并归纳目标目录的各类统计指标（图片总数、子目录数、等级分布、最新图片等）
	Analyze(ctx context.Context, relPath string) (*DirectoryStats, error)
}
