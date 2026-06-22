package hook

import (
	"context"
	"main/internal/scalar"
)

// DispatchMemo 手动派发笔记触发的外部钩子任务
func (h *Handler) DispatchMemo(ctx context.Context, memoRelPath string, hookID scalar.ID) error {
	return h.runner.TriggerForMemo(ctx, memoRelPath, hookID)
}