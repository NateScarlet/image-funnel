package directory

import (
	"context"
	"iter"
)

// Scanner 负责遍历与扫描子目录层级结构
type Scanner interface {
	// Scan 迭代扫描目标目录下的直接子目录结构
	Scan(ctx context.Context, relPath string) iter.Seq2[*Directory, error]
}
