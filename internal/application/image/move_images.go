package image

import (
	"context"
	"main/internal/scalar"
	"main/internal/shared"
	"time"

	"go.uber.org/zap"
)

// MoveImages 移动当前目录中筛选匹配的图片及其配套文件至目标目录中
func (h *Handler) MoveImages(
	ctx context.Context,
	directoryID scalar.ID,
	filterBy shared.ImageFilters,
	toDirectory shared.PathInput,
) (movedCount int, targetAbsDir string, err error) {
	startTime := time.Now()

	dirInfo, err := h.dirSvc.GetDirectory(ctx, directoryID)
	if err != nil {
		return 0, "", err
	}
	relPath := dirInfo.RelPath()

	// 使用领域服务解析并校验输入路径
	targetRelPath, err := h.dirSvc.ResolvePathInput(ctx, relPath, toDirectory)
	if err != nil {
		return 0, "", err
	}

	h.logger.Info("will move images",
		zap.Stringer("directoryID", directoryID),
		zap.String("fromDirectory", relPath),
		zap.String("targetRelPath", targetRelPath),
	)

	movedCount, targetAbsDir, err = h.imgMover.Move(ctx, relPath, filterBy, targetRelPath)
	if err != nil {
		h.logger.Error("move images failed",
			zap.Stringer("directoryID", directoryID),
			zap.String("fromDirectory", relPath),
			zap.String("targetRelPath", targetRelPath),
			zap.Duration("duration", time.Since(startTime)),
			zap.Error(err),
		)
		return 0, "", err
	}

	h.logger.Info("did move images",
		zap.Stringer("directoryID", directoryID),
		zap.String("fromDirectory", relPath),
		zap.String("targetRelPath", targetRelPath),
		zap.Int("movedCount", movedCount),
		zap.String("targetAbsDir", targetAbsDir),
		zap.Duration("duration", time.Since(startTime)),
	)

	return movedCount, targetAbsDir, nil
}
