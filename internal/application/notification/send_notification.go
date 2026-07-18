package notification

import (
	"context"

	"main/internal/scalar"
	"main/internal/shared"

	"go.uber.org/zap"
)

// SendNotification 发送或覆盖通知
func (h *Handler) SendNotification(
	ctx context.Context,
	tag string,
	channel string,
	title string,
	opts ...shared.SendNotificationOption,
) (id scalar.ID, didCreate bool, err error) {
	defer func() {
		if err != nil {
			h.logger.Error("send notification failed",
				zap.String("tag", tag),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did send notification",
				zap.String("tag", tag),
				zap.Bool("didCreate", didCreate),
			)
		}
	}()

	result, err := h.service.SendNotification(ctx, tag, channel, title, opts...)
	if err != nil {
		return scalar.ID{}, false, err
	}

	return result.ID(), result.DidCreate(), nil
}