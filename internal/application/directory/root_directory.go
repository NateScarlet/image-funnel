package directory

import (
	"context"
	"main/internal/domain/relpath"
	"main/internal/shared"
)

// RootDirectory 查询根目录信息
func (h *Handler) RootDirectory(ctx context.Context) (dto *shared.DirectoryDTO, err error) {
	// 根目录是根相对路径的零值
	dir, err := h.repo.Get(ctx, relpath.Path{})
	if err != nil {
		return nil, err
	}
	return h.Directory(ctx, dir.ID())
}
