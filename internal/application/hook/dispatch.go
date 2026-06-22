package hook

import (
	"context"
	"main/internal/scalar"
)

// Dispatch 手动派发特定的外部钩子任务
func (h *Handler) Dispatch(ctx context.Context, ids []string, hookID scalar.ID, triggerName string) error {
	paths, err := h.imageService.GetPaths(ctx, ids)
	if err != nil {
		return err
	}
	return h.runner.Trigger(ctx, ids, paths, hookID, triggerName)
}