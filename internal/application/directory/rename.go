package directory

import (
	"context"
	"main/internal/scalar"
	"time"

	"go.uber.org/zap"
)

// Rename 将目录重命名为同级下的新名称，返回重命名后目录的 ID。
// 目录 ID 由相对路径派生，因此重命名后 ID 会变化，调用方需要用新 ID 继续访问。
func (h *Handler) Rename(ctx context.Context, directoryID scalar.ID, newName string) (newDirectoryID scalar.ID, err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("rename directory failed",
				zap.Stringer("directoryID", directoryID),
				zap.String("newName", newName),
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did rename directory",
				zap.Stringer("directoryID", directoryID),
				zap.Stringer("newDirectoryID", newDirectoryID),
				zap.String("newName", newName),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	h.logger.Info("will rename directory",
		zap.Stringer("directoryID", directoryID),
		zap.String("newName", newName),
	)

	dir, err := h.dirSvc.Rename(ctx, directoryID, newName)
	if err != nil {
		return scalar.ID{}, err
	}
	return dir.ID(), nil
}
