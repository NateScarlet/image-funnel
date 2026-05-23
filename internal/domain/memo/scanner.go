package memo

import (
	"context"
	"iter"
)

// Scanner 负责在指定相对路径目录下扫描匹配的备忘录文件
type Scanner interface {
	// Scan 迭代扫描目录下所有的备忘录信息
	Scan(ctx context.Context, relPath string) iter.Seq2[*Memo, error]
}
