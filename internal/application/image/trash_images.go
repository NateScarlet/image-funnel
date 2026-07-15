package image

import (
	"context"
	"main/internal/scalar"
	"main/internal/shared"
	"time"

	"go.uber.org/zap"
)

// TrashImages 将符合条件的图片移至隐藏的回收站内，并返回生成的历史ID与文件数
func (h *Handler) TrashImages(
	ctx context.Context,
	directoryID scalar.ID,
	filterBy shared.ImageFilters,
	message string,
) (historyId string, totalFileCount int, err error) {
	startTime := time.Now()

	dirInfo, err := h.dirSvc.GetDirectory(ctx, directoryID)
	if err != nil {
		return "", 0, err
	}
	relPath := dirInfo.RelPath()

	h.logger.Info("will trash images",
		zap.Stringer("directoryID", directoryID),
		zap.String("fromDirectory", relPath),
	)

	historyId, totalFileCount, err = h.imgTrasher.Trash(ctx, relPath, filterBy, message)
	if err != nil {
		h.logger.Error("trash images failed",
			zap.Stringer("directoryID", directoryID),
			zap.String("fromDirectory", relPath),
			zap.Duration("duration", time.Since(startTime)),
			zap.Error(err),
		)
		return "", 0, err
	}

	h.logger.Info("did trash images",
		zap.Stringer("directoryID", directoryID),
		zap.String("fromDirectory", relPath),
		zap.String("historyId", historyId),
		zap.Int("totalFileCount", totalFileCount),
		zap.Duration("duration", time.Since(startTime)),
	)

	return historyId, totalFileCount, nil
}