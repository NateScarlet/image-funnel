package image

import (
	"context"
	"main/internal/domain/relpath"
	"main/internal/shared"
)

// ImageByRelPath 通过根相对路径获取图片DTO
func (h *Handler) ImageByRelPath(
	ctx context.Context,
	relPath relpath.Path,
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
