package hook

import (
	"context"
	"main/internal/scalar"
	"time"

	"go.uber.org/zap"
)

// Dispatch 手动派发特定的外部钩子任务
func (h *Handler) Dispatch(ctx context.Context, ids []string, hookID scalar.ID, triggerName string) (err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("dispatch failed",
				zap.Strings("ids", ids),
				zap.Stringer("hookID", hookID),
				zap.String("triggerName", triggerName),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did dispatch",
				zap.Strings("ids", ids),
				zap.Stringer("hookID", hookID),
				zap.String("triggerName", triggerName),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	paths, err := h.imageService.GetPaths(ctx, ids)
	if err != nil {
		return err
	}
	return h.runner.Trigger(ctx, ids, paths, hookID, triggerName)
}
