package notification

import (
	"context"

	"main/internal/scalar"

	"go.uber.org/zap"
)

// UnsendNotification 撤回通知，标记 notAfter 为当前时间
func (h *Handler) UnsendNotification(ctx context.Context, tag string) (id scalar.ID, err error) {
	defer func() {
		if err != nil {
			h.logger.Error("unsend notification failed",
				zap.String("tag", tag),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did unsend notification",
				zap.Stringer("id", id),
				zap.String("tag", tag),
			)
		}
	}()

	return h.service.UnsendNotification(ctx, tag)
}