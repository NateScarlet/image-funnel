package directory

import (
	"context"
	"main/internal/shared"
	"time"

	"go.uber.org/zap"
)

// RootDirectory 查询根目录信息
func (h *Handler) RootDirectory(ctx context.Context) (dto *shared.DirectoryDTO, err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("root directory failed",
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did root directory",
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	dir, err := h.repo.Get(ctx, ".")
	if err != nil {
		return nil, err
	}
	return h.Directory(ctx, dir.ID())
}