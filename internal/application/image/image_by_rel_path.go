package image

import (
	"context"
	"main/internal/shared"
)

// ImageByRelPath 通过相对路径获取图片DTO
func (h *Handler) ImageByRelPath(
	ctx context.Context,
	relPath string,
) (*shared.ImageDTO, error) {
	img, err := h.imageService.ImageByRelPath(ctx, relPath)
	if err != nil {
		return nil, err
	}
	if img == nil {
		return nil, nil
	}
	return h.dtoFactory.New(img)
}