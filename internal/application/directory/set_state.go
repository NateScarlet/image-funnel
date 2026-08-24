package directory

import (
	"context"
	"main/internal/scalar"
	"main/internal/shared"
	"time"

	"go.uber.org/zap"
)

// SetState 设置目录自定义状态
func (h *Handler) SetState(ctx context.Context, id scalar.ID, state *shared.DirectoryStateDTO) (dto *shared.DirectoryDTO, err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("set state failed",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did set state",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	err = h.dirSvc.WriteState(ctx, id, state)
	if err != nil {
		return nil, err
	}
	return h.Directory(ctx, id)
}
