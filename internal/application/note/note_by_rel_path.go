package note

import (
	"context"
	"main/internal/shared"
)

// NoteByRelPath 根据相对路径获取笔记内容
func (h *Handler) NoteByRelPath(ctx context.Context, relPath string) (*shared.NoteDTO, error) {
	n, err := h.service.ReadByRelPath(ctx, relPath)
	if err != nil {
		return nil, err
	}
	return h.dtoFactory.New(n), nil
}
