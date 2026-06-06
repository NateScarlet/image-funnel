package device

import (
	"context"
	"time"

	"main/internal/scalar"
)

// Token 令牌接口
type Token interface {
	String() string
	UserID() scalar.ID
	Expire() time.Time
	IssueAt() time.Time
	// JTI 返回 JWT ID，用于吊销等操作
	JTI() string
}

// TokenSource 令牌源接口，提供标准的双令牌（访问令牌 + 刷新令牌）机制
type TokenSource interface {
	// NewAccessToken 签发一个短效访问令牌，用于高频 API 鉴权
	NewAccessToken(ctx context.Context, deviceID scalar.ID) (Token, error)
	// NewRefreshToken 签发一个长效刷新令牌，用于在访问令牌过期后获取新的令牌对
	NewRefreshToken(ctx context.Context, deviceID scalar.ID) (Token, error)
	// VerifyAccessToken 校验访问令牌，仅验证签名与有效期
	VerifyAccessToken(ctx context.Context, rawToken string) (Token, error)
	// VerifyRefreshToken 校验刷新令牌，验证签名、有效期并检查吊销列表
	VerifyRefreshToken(ctx context.Context, rawToken string) (Token, error)
	// RevokeRefreshToken 吊销指定的刷新令牌，防止重放攻击
	RevokeRefreshToken(ctx context.Context, rawToken string) error
}