package hook

import (
	"context"
	"main/internal/scalar"
	"time"

	"go.uber.org/zap"
)

// DispatchNote 手动派发笔记触发的外部钩子任务
func (h *Handler) DispatchNote(ctx context.Context, noteRelPath string, hookID scalar.ID) (err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("dispatch note failed",
				zap.String("noteRelPath", noteRelPath),
				zap.Stringer("hookID", hookID),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did dispatch note",
				zap.String("noteRelPath", noteRelPath),
				zap.Stringer("hookID", hookID),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	return h.runner.TriggerForNote(ctx, noteRelPath, hookID)
}
