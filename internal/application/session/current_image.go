package session

import (
	"context"
	"main/internal/scalar"
	"main/internal/shared"
)

func (h *Handler) CurrentImage(ctx context.Context, sessionID scalar.ID) (*shared.ImageDTO, error) {
	sess, release, err := h.sessionService.Acquire(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	defer release()

	img := sess.CurrentImage()
	if img == nil {
		return nil, nil
	}

	return h.imageDTOFactory.New(img)
}
