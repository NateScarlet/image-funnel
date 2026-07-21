package directory

import (
	"context"
	"main/internal/scalar"
	"main/internal/shared"
	"time"

	"go.uber.org/zap"
)

// DirectoryStats 查询目录统计信息
func (h *Handler) DirectoryStats(ctx context.Context, id scalar.ID) (dto *shared.DirectoryStatsDTO, err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("directory stats failed",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did directory stats",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	dir, err := h.dirSvc.GetDirectory(ctx, id)
	if err != nil {
		return nil, err
	}
	stats, err := h.dirAnalyzer.Analyze(ctx, dir.RelPath())
	if err != nil {
		return nil, err
	}

	return h.dtoFactory.NewDirectoryStatsDTO(stats)
}