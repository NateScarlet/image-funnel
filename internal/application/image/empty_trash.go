package image

import (
	"context"
	"main/internal/scalar"
	"main/internal/shared"
	"time"

	"go.uber.org/zap"
)

// EmptyTrash 手动清空早于指定保留期限的暂存记录，移入系统回收站
func (h *Handler) EmptyTrash(
	ctx context.Context,
	minAge scalar.Duration,
) (result *shared.EmptyTrashResultDTO, err error) {
	startTime := time.Now()

	stdDuration, err := minAge.Standard()
	if err != nil {
		return nil, err
	}

	h.logger.Info("will empty trash", zap.Duration("minAge", stdDuration))

	// 在 defer 中统一处理成功与失败日志，避免成功与失败路径各写一份
	defer func() {
		if err != nil {
			h.logger.Error("empty trash failed",
				zap.Duration("minAge", stdDuration),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did empty trash",
				zap.Duration("minAge", stdDuration),
				zap.Int("clearedCount", result.ClearedCount),
				zap.Int64("clearedSize", result.ClearedSize),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	result, err = h.imgTrasher.EmptyTrash(ctx, stdDuration)
	return result, err
}
