package concurrency

import (
	"context"

	"main/internal/domain/directory"

	"golang.org/x/sync/singleflight"
)

// SingleFlightDirectoryAnalyzer 负责将针对相同目录的并发统计请求合并为单个物理扫描
type SingleFlightDirectoryAnalyzer struct {
	next  directory.Analyzer
	group singleflight.Group
}

// NewSingleFlightDirectoryAnalyzer 创建 SingleFlight 目录分析装饰器
func NewSingleFlightDirectoryAnalyzer(next directory.Analyzer) *SingleFlightDirectoryAnalyzer {
	return &SingleFlightDirectoryAnalyzer{
		next: next,
	}
}

// Analyze 通过 singleflight 合并相同路径的并发 Analyze 请求
func (a *SingleFlightDirectoryAnalyzer) Analyze(ctx context.Context, relPath string) (*directory.Stats, error) {
	// 使用 context.Background() 运行底层的 Analyze 逻辑，
	// 这样可以避免其中一个调用者的 Context 取消导致共享此请求的其他调用者也返回错误。
	result, err, _ := a.group.Do(relPath, func() (interface{}, error) {
		return a.next.Analyze(context.Background(), relPath)
	})

	if err != nil {
		return nil, err
	}

	return result.(*directory.Stats), nil
}

// 编译时接口检查，确保实现了 directory.Analyzer 接口
var _ directory.Analyzer = (*SingleFlightDirectoryAnalyzer)(nil)
