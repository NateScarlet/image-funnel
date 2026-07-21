package directory

import (
	"context"
	"main/internal/scalar"
	"main/internal/shared"
	"time"

	"go.uber.org/zap"
)

// State 获取目录自定义状态
func (h *Handler) State(ctx context.Context, id scalar.ID) (dto *shared.DirectoryStateDTO, err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("state failed",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did state",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	return h.dirSvc.ReadState(ctx, id)
}