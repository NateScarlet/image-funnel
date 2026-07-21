package directory

import (
	"context"
	"main/internal/scalar"
	"main/internal/shared"
	"time"

	"go.uber.org/zap"
)

// Directory 查询目录信息
func (h *Handler) Directory(ctx context.Context, id scalar.ID) (dto *shared.DirectoryDTO, err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("directory failed",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did directory",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	dirInfo, err := h.dirSvc.GetDirectory(ctx, id)
	if err != nil {
		return nil, err
	}
	return h.dtoFactory.New(dirInfo), nil
}