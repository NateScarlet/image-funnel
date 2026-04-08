package image

import (
	"context"
	"io"
)

type File interface {
	io.ReadSeekCloser
}

type Cache interface {
	// Open 在文件不存在时返回 (nil, nil)
	Open(ctx context.Context, key string) (File, error)
	Save(ctx context.Context, key string, r io.Reader) error
}
