package memo

import (
	"context"
	"main/internal/scalar"
)

// UpdateMemo 更新备忘录
func (h *Handler) UpdateMemo(ctx context.Context, id scalar.ID, content string) error {
	return h.service.Save(ctx, id, content)
}