package memo

import (
	"context"
	"main/internal/scalar"
	"main/internal/shared"
)

// CreateMemo 创建新备忘录文件，若已存在则返回 ALREADY_EXISTS 错误。
func (h *Handler) CreateMemo(ctx context.Context, directoryID scalar.ID, name string, content string) (*shared.MemoDTO, error) {
	dir, err := h.dirSvc.GetDirectory(ctx, directoryID)
	if err != nil {
		return nil, err
	}
	m, err := h.service.Create(ctx, dir.RelPath(), name, content)
	if err != nil {
		return nil, err
	}
	return h.dtoFactory.New(m), nil
}