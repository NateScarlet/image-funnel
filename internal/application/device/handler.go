package device

import (
	"context"
	"errors"
	"iter"
	"net/http"
	"strings"
	"time"

	"main/internal/apperror"
	"main/internal/domain/device"
	"main/internal/scalar"
	"main/internal/shared"
	"main/internal/tokenrw"

	"github.com/go-webauthn/webauthn/protocol"
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

// GenerateToken 为指定设备签发访问令牌和刷新令牌，返回两个令牌引用及其过期时间
func (h *Handler) GenerateToken(ctx context.Context, deviceID scalar.ID) (accessTokenRef string, refreshTokenRef string, accessExpiresAt time.Time, refreshExpiresAt time.Time, err error) {
	accessToken, err := h.tokenSource.NewAccessToken(ctx, deviceID)
	if err != nil {
		return "", "", time.Time{}, time.Time{}, err
	}
	refreshToken, err := h.tokenSource.NewRefreshToken(ctx, deviceID)
	if err != nil {
		return "", "", time.Time{}, time.Time{}, err
	}

	accessTokenRef, err = tokenrw.Write(ctx, accessToken.String(), accessToken.Expire())
	if err != nil {
		return "", "", time.Time{}, time.Time{}, err
	}
	refreshTokenRef, err = tokenrw.Write(ctx, refreshToken.String(), refreshToken.Expire())
	if err != nil {
		return "", "", time.Time{}, time.Time{}, err
	}
	return accessTokenRef, refreshTokenRef, accessToken.Expire(), refreshToken.Expire(), nil
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

// ValidateToken 解析访问令牌，仅验证签名 and 有效期，不检查设备状态以降低性能开销
func (h *Handler) ValidateToken(ctx context.Context, tokenStr string) (scalar.ID, error) {
	rawToken, forget, err := tokenrw.Read(ctx, tokenStr)
	if err != nil {
		return scalar.ID{}, h.formatTokenReadError(err)
	}

	t, err := h.tokenSource.VerifyAccessToken(ctx, rawToken)
	if err != nil {
		forget() // Token is invalid, ask client to forget it
		return scalar.ID{}, err
	}
	return t.UserID(), nil
}

func (h *Handler) AuthStatus(ctx context.Context) (*AuthStatusDTO, error) {
	isTrustedDevice := IsTrustedDevice(ctx)
	isTrustedIP := IsTrustedIP(ctx)

	return &AuthStatusDTO{
		IsTrustedDevice: isTrustedDevice,
		IsTrustedIP:     isTrustedIP,
		CanAccess:       isTrustedDevice || isTrustedIP,
	}, nil
}

type AuthStatusDTO struct {
	IsTrustedDevice bool
	IsTrustedIP     bool
	CanAccess       bool
}

func (h *Handler) BeginWebAuthnRegistration(ctx context.Context) (*protocol.CredentialCreation, string, error) {
	return h.service.BeginRegistration(ctx)
}

func (h *Handler) FinishWebAuthnRegistration(ctx context.Context, sessionKey string, responseJSON string, setupToken string) (device *shared.DeviceDTO, pr *shared.PairingRequestDTO, accessTokenRef string, refreshTokenRef string, accessExpiresAt time.Time, refreshExpiresAt time.Time, err error) {
	startTime := time.Now()
	defer func() {
		if err != nil {
			h.logger.Error("finish webauthn registration failed",
				zap.Duration("duration", time.Since(startTime)),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did finish webauthn registration",
				zap.Duration("duration", time.Since(startTime)),
			)
		}
	}()

	dev, pairingReq, err := h.service.FinishRegistration(ctx, sessionKey, responseJSON, setupToken, IsTrustedIP(ctx), RemoteIP(ctx), UserAgent(ctx))
	if err != nil {
		return nil, nil, "", "", time.Time{}, time.Time{}, err
	}

	var deviceDTO *shared.DeviceDTO
	var accessExp, refreshExp time.Time
	if dev != nil {
		accessTokenRef, refreshTokenRef, accessExp, refreshExp, err = h.GenerateToken(ctx, dev.ID())
		if err != nil {
			return nil, nil, "", "", time.Time{}, time.Time{}, err
		}
		deviceDTO = h.dtoFactory.New(dev)
	}

	var prDTO *shared.PairingRequestDTO
	if pairingReq != nil {
		prDTO = &shared.PairingRequestDTO{
			Code:      pairingReq.Code(),
			CreatedAt: pairingReq.CreatedAt(),
			Status:    shared.PairingRequestStatusPending,
		}
	}

	return deviceDTO, prDTO, accessTokenRef, refreshTokenRef, accessExp, refreshExp, nil
}

func (h *Handler) BeginWebAuthnLogin(ctx context.Context) (*protocol.CredentialAssertion, string, error) {
	return h.service.BeginLogin(ctx)
}

func (h *Handler) FinishWebAuthnLogin(ctx context.Context, sessionKey string, responseJSON string, ip string) (accessTokenRef string, refreshTokenRef string, accessExpiresAt time.Time, refreshExpiresAt time.Time, device *shared.DeviceDTO, err error) {
	startTime := time.Now()
	defer func() {
		if err != nil {
			h.logger.Error("finish webauthn login failed",
				zap.Duration("duration", time.Since(startTime)),
				zap.String("ip", ip),
				zap.Error(err),
			)
		} else {
			h.logger.Info("did finish webauthn login",
				zap.Duration("duration", time.Since(startTime)),
				zap.String("ip", ip),
			)
		}
	}()

	dev, err := h.service.FinishLogin(ctx, sessionKey, responseJSON, ip, UserAgent(ctx))
	if err != nil {
		return "", "", time.Time{}, time.Time{}, nil, err
	}
	accessTokenRef, refreshTokenRef, accessExpiresAt, refreshExpiresAt, err = h.GenerateToken(ctx, dev.ID())
	if err != nil {
		return "", "", time.Time{}, time.Time{}, nil, err
	}
	device = h.dtoFactory.New(dev)
	return accessTokenRef, refreshTokenRef, accessExpiresAt, refreshExpiresAt, device, nil
}

func (h *Handler) List(ctx context.Context) ([]*shared.DeviceDTO, error) {
	devices, err := h.service.List(ctx)
	if err != nil {
		return nil, err
	}
	var dtos []*shared.DeviceDTO
	for _, d := range devices {
		dtos = append(dtos, h.dtoFactory.New(d))
	}
	return dtos, nil
}

func (h *Handler) Delete(ctx context.Context, id scalar.ID) error {
	return h.service.Delete(ctx, id)
}

func (h *Handler) RefreshToken(ctx context.Context, tokenStr string) (accessTokenRef string, refreshTokenRef string, accessExpiresAt time.Time, refreshExpiresAt time.Time, err error) {
	if tokenStr == "" {
		return "", "", time.Time{}, time.Time{}, apperror.New("UNAUTHORIZED", "unauthorized access", "未授权访问")
	}
	rawToken, forget, err := tokenrw.Read(ctx, tokenStr)
	if err != nil {
		return "", "", time.Time{}, time.Time{}, h.formatTokenReadError(err)
	}

	// 吊销当前刷新令牌，防止重放攻击
	if revokeErr := h.tokenSource.RevokeRefreshToken(ctx, rawToken); revokeErr != nil {
		h.logger.Warn("failed to revoke refresh token", zap.Error(revokeErr))
	}

	t, err := h.tokenSource.VerifyRefreshToken(ctx, rawToken)
	if err != nil {
		forget()
		return "", "", time.Time{}, time.Time{}, err
	}

	deviceID := t.UserID()

	// 仅在刷新时检查设备是否仍为可信设备
	hasDevice, err := h.service.Exists(ctx, deviceID)
	if err != nil || !hasDevice {
		forget()
		return "", "", time.Time{}, time.Time{}, apperror.New("UNAUTHORIZED", "device deleted", "设备已被删除")
	}

	// 签发新令牌对
	accessToken, err := h.tokenSource.NewAccessToken(ctx, deviceID)
	if err != nil {
		forget()
		return "", "", time.Time{}, time.Time{}, err
	}
	refreshToken, err := h.tokenSource.NewRefreshToken(ctx, deviceID)
	if err != nil {
		forget()
		return "", "", time.Time{}, time.Time{}, err
	}

	// 将新刷新令牌的 JTI 写入设备，确保设备移除时可自动吊销
	if updateErr := h.service.UpdateRefreshToken(ctx, deviceID, refreshToken.JTI(), refreshToken.Expire()); updateErr != nil {
		h.logger.Warn("failed to update device refresh token id", zap.Error(updateErr))
	}

	accessTokenRef, err = tokenrw.Write(ctx, accessToken.String(), accessToken.Expire())
	if err != nil {
		forget()
		return "", "", time.Time{}, time.Time{}, err
	}
	refreshTokenRef, err = tokenrw.Write(ctx, refreshToken.String(), refreshToken.Expire())
	if err != nil {
		forget()
		return "", "", time.Time{}, time.Time{}, err
	}

	forget()
	return accessTokenRef, refreshTokenRef, accessToken.Expire(), refreshToken.Expire(), nil
}

func (h *Handler) SubscribeSaved(ctx context.Context) iter.Seq2[*shared.DeviceDTO, error] {
	if h.ebus != nil {
		return h.ebus.SubscribeDeviceSaved(ctx)
	}
	return func(yield func(*shared.DeviceDTO, error) bool) {}
}

func (h *Handler) SubscribeDeleted(ctx context.Context) iter.Seq2[scalar.ID, error] {
	if h.ebus != nil {
		return h.ebus.SubscribeDeviceDeleted(ctx)
	}
	return func(yield func(scalar.ID, error) bool) {}
}

// ---------------- Context utilities ----------------

type contextKey string

const (
	trustedDeviceKey contextKey = "trustedDevice"
	trustedIPKey     contextKey = "trustedIP"
	remoteIPKey      contextKey = "remoteIP"
	userAgentKey     contextKey = "userAgent"
	rawTokenKey      contextKey = "rawToken"
)

func WithTrustedDevice(ctx context.Context, deviceID scalar.ID) context.Context {
	return context.WithValue(ctx, trustedDeviceKey, deviceID)
}

func IsTrustedDevice(ctx context.Context) bool {
	_, ok := ctx.Value(trustedDeviceKey).(scalar.ID)
	return ok
}

func TrustedDeviceID(ctx context.Context) (scalar.ID, bool) {
	id, ok := ctx.Value(trustedDeviceKey).(scalar.ID)
	return id, ok
}

func WithTrustedIP(ctx context.Context, isTrusted bool) context.Context {
	return context.WithValue(ctx, trustedIPKey, isTrusted)
}

func IsTrustedIP(ctx context.Context) bool {
	v, ok := ctx.Value(trustedIPKey).(bool)
	return ok && v
}

func WithRemoteIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, remoteIPKey, ip)
}

func RemoteIP(ctx context.Context) string {
	v, ok := ctx.Value(remoteIPKey).(string)
	if !ok {
		return "unknown"
	}
	return v
}

func WithUserAgent(ctx context.Context, ua string) context.Context {
	return context.WithValue(ctx, userAgentKey, ua)
}

func UserAgent(ctx context.Context) string {
	v, ok := ctx.Value(userAgentKey).(string)
	if !ok {
		return ""
	}
	return v
}
