package device

import (
	"context"
	"main/internal/scalar"
	"main/internal/tokenrw"
)

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
