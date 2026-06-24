package note

import (
	"context"
	"iter"
)

// Repository 定义了笔记信息的存储接口
type Repository interface {
	// Read 读取指定相对路径的笔记信息，如果不存在则返回 nil, nil
	Read(ctx context.Context, relPath string) (*Note, error)
	// Write 写入指定相对路径的笔记内容，如果 content 为空则删除对应文件
	Write(ctx context.Context, relPath string, content string) error
	// Find 迭代扫描目录下所有的笔记信息
	Find(ctx context.Context, relPath string) iter.Seq2[*Note, error]
}
