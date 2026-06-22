package session

import (
	"context"
	"main/internal/scalar"
	"main/internal/shared"
)

func (h *Handler) KeptImages(ctx context.Context, sessionID scalar.ID, limit, offset int) ([]*shared.ImageDTO, error) {
	sess, release, err := h.sessionService.Acquire(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	defer release()

	images := sess.KeptImages(limit, offset)
	if len(images) == 0 {
		return nil, nil
	}

	result := make([]*shared.ImageDTO, 0, len(images))
	for _, img := range images {
		dto, err := h.imageDTOFactory.New(img)
		if err != nil {
			return nil, err
		}
		result = append(result, dto)
	}
	return result, nil
}