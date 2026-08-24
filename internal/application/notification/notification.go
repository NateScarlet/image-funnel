package notification

import (
	"context"

	"main/internal/apperror"
	"main/internal/scalar"
	"main/internal/shared"
)

// Notification 获取单条通知
func (h *Handler) Notification(ctx context.Context, id scalar.ID) (dto *shared.NotificationDTO, err error) {
	if id.IsZero() {
		return nil, apperror.NewErrDocumentNotFound(id)
	}
	notif, err := h.repo.Get(ctx, id.String())
	if err != nil {
		return nil, err
	}
	return h.dtoFactory.New(notif), nil
}
