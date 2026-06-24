package note

import (
	"context"
	"main/internal/scalar"
	"main/internal/shared"
)

// Note 获取笔记内容
func (h *Handler) Note(ctx context.Context, id scalar.ID) (*shared.NoteDTO, error) {
	n, err := h.service.Read(ctx, id)
	if err != nil {
		return nil, err
	}
	return h.dtoFactory.New(n), nil
}
