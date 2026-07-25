package image

import (
	"context"
	"main/internal/scalar"
	"path/filepath"
	"strings"
)

// ComfyUIWorkflow 通过图片 ID 获取 ComfyUI 工作流
func (h *Handler) ComfyUIWorkflow(
	ctx context.Context,
	id scalar.ID,
) (_ *string, err error) {
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
