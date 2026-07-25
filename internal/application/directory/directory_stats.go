package directory

import (
	"context"
	"main/internal/scalar"
	"main/internal/shared"
)

// DirectoryStats 查询目录统计信息
func (h *Handler) DirectoryStats(ctx context.Context, id scalar.ID) (dto *shared.DirectoryStatsDTO, err error) {
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
