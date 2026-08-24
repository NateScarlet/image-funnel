package image

import (
	"context"
	"main/internal/scalar"
	"main/internal/shared"
)

// Image 通过 ID 获取图片
func (h *Handler) Image(
	ctx context.Context,
	id scalar.ID,
) (*shared.ImageDTO, error) {
	img, err := h.imageService.GetImage(ctx, id)
	if err != nil {
		return nil, err
	}
	if img == nil {
		return nil, nil
	}
	return h.dtoFactory.New(img)
}
