package memo

import (
	"context"
	"main/internal/scalar"
)

// Repository 定义了备忘信息的存储接口
type Repository interface {
	// Read 读取指定 ID 的备忘信息，如果不存在则返回 nil, nil
	Read(ctx context.Context, id scalar.ID) (*Memo, error)
	// Write 写入指定 ID 的备忘内容，如果 content 为空则删除对应文件
	Write(ctx context.Context, id scalar.ID, content string) error
}
