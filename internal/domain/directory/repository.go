package directory

import (
	"context"
	"iter"
)

type Repository interface {
	Get(ctx context.Context, relPath string) (*Directory, error)
	// Find 迭代扫描目标目录下的直接子目录结构
	Find(ctx context.Context, relPath string) iter.Seq2[*Directory, error]
}
