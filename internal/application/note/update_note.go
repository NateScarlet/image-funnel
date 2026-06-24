package note

import (
	"context"
	"main/internal/scalar"
)

// UpdateNote 更新笔记
func (h *Handler) UpdateNote(ctx context.Context, id scalar.ID, content string) error {
	return h.service.Save(ctx, id, content)
}
