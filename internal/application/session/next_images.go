package session

import (
	"context"
	"main/internal/scalar"
	"main/internal/shared"
)

func (h *Handler) NextImages(ctx context.Context, sessionID scalar.ID, count int) ([]*shared.ImageDTO, error) {
	sess, release, err := h.sessionService.Acquire(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	defer release()

	images := sess.NextImages(count)
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