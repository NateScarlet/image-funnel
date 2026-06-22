package directory

import (
	"context"
	"main/internal/shared"
)

// RootDirectory 查询根目录信息
func (h *Handler) RootDirectory(ctx context.Context) (*shared.DirectoryDTO, error) {
	dir, err := h.repo.Get(ctx, ".")
	if err != nil {
		return nil, err
	}
	return h.Directory(ctx, dir.ID())
}