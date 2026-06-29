package image

import (
	"context"
	"main/internal/shared"
	"time"

	"go.uber.org/zap"
)

// UpdateImagesMetadata 批量更新符合条件的图片的元数据，操作即时写入 XMP 伴随文件
func (h *Handler) UpdateImagesMetadata(
	ctx context.Context,
	filterBy shared.ImageFilters,
	rating *int,
	label *string,
) (updatedCount int, err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("batch update images metadata failed",
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did batch update images metadata",
				zap.Int("updatedCount", updatedCount),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	matched, err := h.FindMatchedImages(ctx, filterBy)
	if err != nil {
		return 0, err
	}

	for _, img := range matched {
		err = h.imageService.UpdateImageMetadata(ctx, img.ID(), rating, label)
		if err != nil {
			return updatedCount, err
		}
		updatedCount++
	}

	return updatedCount, nil
}
