package device

import (
	"context"
	"main/internal/scalar"
)

func (h *Handler) Delete(ctx context.Context, id scalar.ID) error {
	return h.service.Delete(ctx, id)
}