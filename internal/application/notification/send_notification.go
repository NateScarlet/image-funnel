package notification

import (
	"context"

	"main/internal/shared"

	"go.uber.org/zap"
)

// Send 发送或覆盖通知
func (h *Handler) Send(
	ctx context.Context,
	tag string,
	channel string,
	title string,
	opts ...shared.SendNotificationOption,
) (result *shared.SendNotificationResult, err error) {
	defer func() {
		if err != nil {
			h.logger.Error("send notification failed",
				zap.String("tag", tag),
				zap.String("channel", channel),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did send notification",
				zap.Stringer("id", result.ID()),
				zap.String("tag", tag),
				zap.Bool("didCreate", result.DidCreate()),
			)
		}
	}()

	return h.service.Send(ctx, tag, channel, title, opts...)
}
