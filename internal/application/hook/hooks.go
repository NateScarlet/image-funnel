package hook

import (
	"context"
	"main/internal/shared"
)

// Hooks 获取所有外部钩子概要列表
func (h *Handler) Hooks(ctx context.Context) ([]*shared.HookDTO, error) {
	hooks, err := h.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	var res []*shared.HookDTO
	for _, hk := range hooks {
		res = append(res, h.dtoFactory.New(hk))
	}
	return res, nil
}