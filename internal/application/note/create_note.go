package note

import (
	"context"
	"main/internal/scalar"
	"main/internal/shared"
)

// CreateNote 创建新笔记文件，若已存在则返回 ALREADY_EXISTS 错误。
func (h *Handler) CreateNote(ctx context.Context, directoryID scalar.ID, name string, content string) (*shared.NoteDTO, error) {
	dir, err := h.dirSvc.GetDirectory(ctx, directoryID)
	if err != nil {
		return nil, err
	}
	n, err := h.service.Create(ctx, dir.RelPath(), name, content)
	if err != nil {
		return nil, err
	}
	return h.dtoFactory.New(n), nil
}
