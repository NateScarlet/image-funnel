package note

import (
	"context"
	"main/internal/shared"
	"time"

	"go.uber.org/zap"
)

// NoteByRelPath 根据相对路径获取笔记内容
func (h *Handler) NoteByRelPath(ctx context.Context, relPath string) (dto *shared.NoteDTO, err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("note by rel path failed",
				zap.String("relPath", relPath),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did note by rel path",
				zap.String("relPath", relPath),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	n, err := h.service.ReadByRelPath(ctx, relPath)
	if err != nil {
		return nil, err
	}
	return h.dtoFactory.New(n), nil
}
