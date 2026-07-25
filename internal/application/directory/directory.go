package directory

import (
	"context"
	"main/internal/scalar"
	"main/internal/shared"
)

// Directory 查询目录信息
func (h *Handler) Directory(ctx context.Context, id scalar.ID) (dto *shared.DirectoryDTO, err error) {
	dirInfo, err := h.dirSvc.GetDirectory(ctx, id)
	if err != nil {
		return nil, err
	}
	return h.dtoFactory.New(dirInfo), nil
}
