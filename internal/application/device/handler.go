package device

import (
	"errors"
	"net/http"
	"strings"

	"main/internal/apperror"
	"main/internal/domain/device"
	"main/internal/pubsub"
	"main/internal/scalar"
	"main/internal/tokenrw"

	"go.uber.org/zap"
)

type Handler struct {
	service         *device.Service
	tokenSource     device.TokenSource
	dtoFactory      *DTOFactory
	logger          *zap.Logger
	deviceSavedSub  pubsub.Topic[*device.Device]
	deviceDeletedSub pubsub.Topic[scalar.ID]
}

func NewHandler(
	service *device.Service,
	tokenSource device.TokenSource,
	dtoFactory *DTOFactory,
	logger *zap.Logger,
	deviceSavedSub pubsub.Topic[*device.Device],
	deviceDeletedSub pubsub.Topic[scalar.ID],
) *Handler {
	return &Handler{
		service:         service,
		tokenSource:     tokenSource,
		dtoFactory:      dtoFactory,
		logger:          logger,
		deviceSavedSub:  deviceSavedSub,
		deviceDeletedSub: deviceDeletedSub,
	}
}

// formatTokenReadError 将读取令牌时发生的底层错误（如 Cookie 缺失、Cookie 无法读取等）包装为前端可识别的无效令牌错误
func (h *Handler) formatTokenReadError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, http.ErrNoCookie) {
		return apperror.New(
			"INVALID_TOKEN",
			"token cookie not present or expired",
			"令牌已失效或过期",
		)
	}
	if errors.Is(err, tokenrw.ErrCookiesUnavailable) {
		return apperror.New(
			"INVALID_TOKEN",
			"cookies unavailable",
			"无法读取 Cookie 令牌",
		)
	}
	if strings.HasPrefix(err.Error(), "tokenrw:") {
		return apperror.New(
			"INVALID_TOKEN",
			err.Error(),
			"令牌读取失败",
		)
	}
	return err
}