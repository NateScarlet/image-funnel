package device

import (
	"context"
	"errors"
	"iter"
	"net/http"
	"strings"

	"main/internal/apperror"
	"main/internal/domain/device"
	"main/internal/scalar"
	"main/internal/shared"
	"main/internal/tokenrw"

	"go.uber.org/zap"
)

type EventBus interface {
	SubscribeDeviceSaved(ctx context.Context) iter.Seq2[*shared.DeviceDTO, error]
	SubscribeDeviceDeleted(ctx context.Context) iter.Seq2[scalar.ID, error]
}

type Handler struct {
	service     *device.Service
	tokenSource device.TokenSource
	dtoFactory  *DTOFactory
	logger      *zap.Logger
	ebus        EventBus
}

func NewHandler(service *device.Service, tokenSource device.TokenSource, dtoFactory *DTOFactory, logger *zap.Logger, ebus EventBus) *Handler {
	return &Handler{
		service:     service,
		tokenSource: tokenSource,
		dtoFactory:  dtoFactory,
		logger:      logger,
		ebus:        ebus,
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