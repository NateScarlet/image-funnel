package image

import (
	"context"
	"io"
)

// File 代表缓存中的一个文件句柄。它本身不是可读流，
// 必须调用 Open() 获得独立的 io.ReadSeekCloser 进行读取。
type File interface {
	// Open 返回一个独立的读取器（从文件开头开始）。
	// 每个调用者获得的 Reader 有自己的偏移量，可以安全并发使用。
	// 调用方负责 Close 返回的 Reader。
	Open() (io.ReadSeekCloser, error)
}

// Cache 提供基于 key 的文件缓存操作。
type Cache interface {
	// Lookup 根据 key 获取文件句柄。如果文件不存在，返回 (nil, nil)。
	Lookup(ctx context.Context, key string) (File, error)

	// Save 将 r 中的内容保存到 key 对应的缓存文件中。
	Save(ctx context.Context, key string, r io.Reader) error
}
