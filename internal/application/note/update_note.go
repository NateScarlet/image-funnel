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

	if err := h.service.Save(ctx, id, content); err != nil {
		return err
	}

	// 写入完成后登记保存完成事件，由钩子链路终结后统一发布
	n, err := h.service.Read(ctx, id)
	if err != nil {
		return err
	}
	h.savedEvents.Saved(ctx, n.RelPath())
	return nil
}
