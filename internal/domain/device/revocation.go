package device

import (
	"context"
	"time"
)

// RevocationList 管理刷新令牌的吊销状态，防止重放攻击
type RevocationList interface {
	// Add 将指定 JTI 加入吊销列表，expiresAt 用于清理过期条目
	Add(ctx context.Context, jti string, expiresAt time.Time) error
	// IsRevoked 检查指定 JTI 是否已被吊销
	IsRevoked(ctx context.Context, jti string) (bool, error)
}