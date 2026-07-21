package note

import (
	"context"
	"main/internal/scalar"
	"main/internal/shared"
	"time"

	"go.uber.org/zap"
)

// CreateNote 创建新笔记文件，若已存在则返回 ALREADY_EXISTS 错误。
func (h *Handler) CreateNote(ctx context.Context, directoryID scalar.ID, name string, content string) (dto *shared.NoteDTO, err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("create note failed",
				zap.Stringer("directoryID", directoryID),
				zap.String("name", name),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did create note",
				zap.Stringer("id", dto.ID),
				zap.Stringer("directoryID", directoryID),
				zap.String("name", name),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	dir, err := h.dirSvc.GetDirectory(ctx, directoryID)
	if err != nil {
		return nil, err
	}
	n, err := h.service.Create(ctx, dir.RelPath(), name, content)
	if err != nil {
		return nil, err
	}
	return h.dtoFactory.New(n), nil
}
