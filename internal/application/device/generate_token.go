package device

import (
	"context"
	"main/internal/scalar"
	"main/internal/tokenrw"
	"time"
)

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
