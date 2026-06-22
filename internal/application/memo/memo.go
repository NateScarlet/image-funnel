package memo

import (
	"context"
	"main/internal/scalar"
	"main/internal/shared"
)

// Memo 获取备忘录内容
func (h *Handler) Memo(ctx context.Context, id scalar.ID) (*shared.MemoDTO, error) {
	m, err := h.service.Read(ctx, id)
	if err != nil {
		return nil, err
	}
	return h.dtoFactory.New(m), nil
}