package image

import (
	"context"
	"main/internal/scalar"
	"time"

	"go.uber.org/zap"
)

// UpdateImageMetadata 更新单个图片的元数据（评星和颜色标签），操作即时写入 XMP 伴随文件
func (h *Handler) UpdateImageMetadata(
	ctx context.Context,
	id scalar.ID,
	rating *int,
	label *string,
) (err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("update image metadata failed",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did update image metadata",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	return h.imageService.UpdateImageMetadata(ctx, id, rating, label)
}