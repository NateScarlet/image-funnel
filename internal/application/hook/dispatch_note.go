package hook

import (
	"context"
	"main/internal/scalar"
)

// DispatchNote 手动派发笔记触发的外部钩子任务
func (h *Handler) DispatchNote(ctx context.Context, noteRelPath string, hookID scalar.ID) error {
	return h.runner.TriggerForNote(ctx, noteRelPath, hookID)
}
