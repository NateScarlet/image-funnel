package note

import (
	"context"
	"main/internal/scalar"
	"main/internal/shared"
	"time"

	"go.uber.org/zap"
)

// Note 获取笔记内容
func (h *Handler) Note(ctx context.Context, id scalar.ID) (dto *shared.NoteDTO, err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("note failed",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did note",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	n, err := h.service.Read(ctx, id)
	if err != nil {
		return nil, err
	}
	return h.dtoFactory.New(n), nil
}
