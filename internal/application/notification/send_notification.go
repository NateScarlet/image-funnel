package notification

import (
	"context"

	"main/internal/shared"

	"go.uber.org/zap"
)

// SendNotification 发送或覆盖通知
func (h *Handler) SendNotification(
	ctx context.Context,
	channel string,
	title string,
	opts ...shared.SendNotificationOption,
) (result *shared.SendNotificationResult, err error) {
	options := shared.NewSendNotificationOptions(opts...)
	tag := options.Tag()
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

	return h.service.Send(ctx, channel, title, opts...)
}

var _ shared.NotificationSender = (*Handler)(nil)
