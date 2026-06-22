package image

import (
	"context"
	"main/internal/scalar"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

// ComfyUIWorkflow 通过图片 ID 获取 ComfyUI 工作流
func (h *Handler) ComfyUIWorkflow(
	ctx context.Context,
	id scalar.ID,
) (_ *string, err error) {
	startTime := time.Now()

	defer func() {
		if err != nil {
			h.logger.Error("get ComfyUI workflow failed",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
				zap.Any("err", err),
			)
		} else {
			h.logger.Debug("did get ComfyUI workflow",
				zap.Stringer("id", id),
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	img, err := h.imageService.GetImage(ctx, id)
	if err != nil {
		return nil, err
	}

	ext := strings.ToLower(filepath.Ext(img.Filename()))
	if ext != ".png" {
		return nil, nil
	}

	workflow, err := ExtractComfyUIWorkflow(filepath.Join(h.rootDir, img.RelPath()))
	if err != nil {
		return nil, err
	}

	return workflow, nil
}