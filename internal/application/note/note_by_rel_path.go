package note

import (
	"context"
	"main/internal/domain/relpath"
	"main/internal/shared"
)

// NoteByRelPath 根据根相对路径获取笔记内容
func (h *Handler) NoteByRelPath(ctx context.Context, relPath relpath.Path) (dto *shared.NoteDTO, err error) {
	n, err := h.service.ReadByRelPath(ctx, relPath)
	if err != nil {
		return nil, err
	}
	return h.dtoFactory.New(n), nil
}
