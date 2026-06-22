package memo

import (
	"context"
	"main/internal/shared"
)

// MemoByRelPath 根据相对路径获取备忘录内容
func (h *Handler) MemoByRelPath(ctx context.Context, relPath string) (*shared.MemoDTO, error) {
	m, err := h.service.ReadByRelPath(ctx, relPath)
	if err != nil {
		return nil, err
	}
	return h.dtoFactory.New(m), nil
}