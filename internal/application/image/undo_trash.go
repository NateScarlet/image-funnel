package image

import (
	"context"
	"main/internal/shared"
	"time"

	"go.uber.org/zap"
)

// UndoTrash 撤销指定的回收站移动操作，还原文件
func (h *Handler) UndoTrash(
	ctx context.Context,
	historyId string,
) (result *shared.UndoTrashResultDTO, err error) {
	startTime := time.Now()

	h.logger.Info("will undo trash", zap.String("historyId", historyId))

	result, err = h.imgTrasher.UndoTrash(ctx, historyId)
	if err != nil {
		h.logger.Error("undo trash failed",
			zap.String("historyId", historyId),
			zap.Duration("duration", time.Since(startTime)),
			zap.Error(err),
		)
		return nil, err
	}

	h.logger.Info("did undo trash",
		zap.String("historyId", historyId),
		zap.Int("restoredCount", result.RestoredCount),
		zap.Int("conflictCount", result.ConflictCount),
		zap.String("conflictDirName", result.ConflictDirName),
		zap.Duration("duration", time.Since(startTime)),
	)

	return result, nil
}