package device

import (
	"context"
	"errors"
	"main/internal/apperror"
	"main/internal/domain/device"
	"main/internal/tokenrw"
	"time"

	"go.uber.org/zap"
)

func (h *Handler) RefreshToken(ctx context.Context, tokenStr string) (accessTokenRef string, refreshTokenRef string, accessExpiresAt time.Time, refreshExpiresAt time.Time, err error) {
	if tokenStr == "" {
		return "", "", time.Time{}, time.Time{}, apperror.New("UNAUTHORIZED", "unauthorized access", "未授权访问")
	}
	rawToken, forget, err := tokenrw.Read(ctx, tokenStr)
	if err != nil {
		return "", "", time.Time{}, time.Time{}, h.formatTokenReadError(err)
	}

	// 校验刷新令牌的有效性（包括签名、有效期和当时的吊销状态）
	t, err := h.tokenSource.VerifyRefreshToken(ctx, rawToken)
	if err != nil {
		forget()
		return "", "", time.Time{}, time.Time{}, err
	}

	deviceID := t.UserID()

	// 仅在刷新时检查设备是否仍为可信设备。如果设备已被删除，后续校验都会直接因设备不存在而失败，
	// 此时无需将令牌加入吊销列表，以避免往吊销列表中写入已删除设备关联的无效数据。
	hasDevice, err := h.service.Exists(ctx, deviceID)
	if err != nil || !hasDevice {
		forget()
		return "", "", time.Time{}, time.Time{}, apperror.New("UNAUTHORIZED", "device deleted", "设备已被删除")
	}

	// 准备撤销刷新令牌，防止此令牌被二次重放攻击
	revokeFn, err := h.service.PrepareRevoke(ctx, t.JTI(), t.Expire())
	if err != nil {
		forget()
		if errors.Is(err, device.ErrTokenAlreadyRevoked) {
			return "", "", time.Time{}, time.Time{}, apperror.New(
				"INVALID_TOKEN",
				"refresh token has been revoked",
				"刷新令牌已被吊销",
			)
		}
		return "", "", time.Time{}, time.Time{}, err
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

	// 正式吊销刷新令牌
	if err := revokeFn(); err != nil {
		forget()
		if errors.Is(err, device.ErrTokenAlreadyRevoked) {
			return "", "", time.Time{}, time.Time{}, apperror.New(
				"INVALID_TOKEN",
				"refresh token has been revoked",
				"刷新令牌已被吊销",
			)
		}
		return "", "", time.Time{}, time.Time{}, err
	}

	// 将新刷新令牌的 JTI 写入设备，确保设备移除时可自动吊销
	if updateErr := h.service.UpdateRefreshToken(ctx, deviceID, refreshToken.JTI(), refreshToken.Expire(), RemoteIP(ctx), UserAgent(ctx)); updateErr != nil {
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
