package directory

import (
	"context"
	"main/internal/scalar"
	"main/internal/shared"
)

// State 获取目录自定义状态
func (h *Handler) State(ctx context.Context, id scalar.ID) (dto *shared.DirectoryStateDTO, err error) {
	return h.dirSvc.ReadState(ctx, id)
}
