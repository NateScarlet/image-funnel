package image

import (
	"context"
	"io"

	"main/internal/shared/transcode"
)

// Cache 提供基于 key 的文件缓存操作。
type Cache interface {
	// Lookup 根据 key 获取文件句柄。如果文件不存在，返回 (nil, nil)。
	Lookup(ctx context.Context, key string) (transcode.File, error)

	// Save 将 r 中的内容保存到 key 对应的缓存文件中。
	Save(ctx context.Context, key string, r io.Reader) error
}
