package image

import (
	"context"
	"main/internal/scalar"
	"time"

	"go.uber.org/zap"
)

// EmptyTrash 手动清空早于指定保留期限的暂存记录，移入系统回收站
func (h *Handler) EmptyTrash(
	ctx context.Context,
	minAge scalar.Duration,
) (clearedCount int, err error) {
	startTime := time.Now()

	stdDuration, err := minAge.Standard()
	if err != nil {
		return 0, err
	}

	h.logger.Info("will empty trash", zap.Duration("minAge", stdDuration))

	clearedCount, err = h.imgTrasher.EmptyTrash(ctx, stdDuration)
	if err != nil {
		h.logger.Error("empty trash failed",
			zap.Duration("minAge", stdDuration),
			zap.Duration("duration", time.Since(startTime)),
			zap.Error(err),
		)
		return 0, err
	}

	h.logger.Info("did empty trash",
		zap.Duration("minAge", stdDuration),
		zap.Int("clearedCount", clearedCount),
		zap.Duration("duration", time.Since(startTime)),
	)

	return clearedCount, nil
}