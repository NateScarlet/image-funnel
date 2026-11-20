package session

import (
	"context"
	"main/internal/domain/directory"
	"main/internal/domain/relpath"
	"main/internal/scalar"
)

// FakeDirectoryResolver 解析目录的替身；Err 非零时模拟解析失败，零值仍返回根目录。
type FakeDirectoryResolver struct {
	Err error
}

func (r *FakeDirectoryResolver) GetDirectory(ctx context.Context, id scalar.ID) (*directory.Directory, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	// 根目录是根相对路径的零值
	return directory.FromRepository(relpath.Path{}), nil
}
