package note

import (
	"context"
	"main/internal/scalar"
	"time"

	"go.uber.org/zap"
)

// UpdateNote 更新笔记
func (h *Handler) UpdateNote(ctx context.Context, id scalar.ID, content string) (err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("update note failed",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did update note",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	return h.service.Save(ctx, id, content)
}
