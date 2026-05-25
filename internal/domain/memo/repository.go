package memo

import (
	"context"
)

// Repository 定义了备忘信息的存储接口
type Repository interface {
	// Read 读取指定相对路径的备忘信息，如果不存在则返回 nil, nil
	Read(ctx context.Context, relPath string) (*Memo, error)
	// Write 写入指定相对路径的备忘内容，如果 content 为空则删除对应文件
	Write(ctx context.Context, relPath string, content string) error
}
