package directory

import (
	"context"
	"main/internal/scalar"
	"main/internal/shared"
)

// SetState 设置目录自定义状态
func (h *Handler) SetState(ctx context.Context, id scalar.ID, state *shared.DirectoryStateDTO) (*shared.DirectoryDTO, error) {
	err := h.dirSvc.WriteState(ctx, id, state)
	if err != nil {
		return nil, err
	}
	return h.Directory(ctx, id)
}