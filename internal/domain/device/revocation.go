package device

import (
	"context"
	"time"

	"main/internal/apperror"
)

var ErrTokenAlreadyRevoked = apperror.New(
	"TOKEN_REVOKED",
	"token is revoked",
	"令牌已被撤销",
)

type RevokeFunc func() error

// RevocationList 管理刷新令牌的吊销状态，防止重放攻击
type RevocationList interface {
	// PrepareRevoke 原子性地两步提交撤销一个令牌。
	// 如果令牌已被撤销（已存在），返回 ErrTokenAlreadyRevoked。
	// 如果成功撤销，返回 nil。
	// 其他错误（如数据库错误）也通过 error 返回。
	PrepareRevoke(ctx context.Context, id string, expiresAt time.Time) (RevokeFunc, error)
}
