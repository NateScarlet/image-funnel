package jwt

import (
	"context"
	"errors"
	"fmt"
	"time"

	"main/internal/apperror"
	"main/internal/domain/device"
	"main/internal/scalar"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var _ device.TokenSource = (*TokenSource)(nil)

// TokenSource 提供基于 JWT 的双令牌（访问令牌 + 刷新令牌）生成和校验功能
type TokenSource struct {
	accessTokenLife  time.Duration
	refreshTokenLife time.Duration
	secret           []byte
	signingMethod    jwt.SigningMethod
}

// NewTokenSource 构造一个新的 TokenSource 实例
func NewTokenSource(
	accessTokenLife time.Duration,
	refreshTokenLife time.Duration,
	secret []byte,
) *TokenSource {
	if secret == nil {
		panic("secret is required")
	}
	return &TokenSource{
		accessTokenLife:  accessTokenLife,
		refreshTokenLife: refreshTokenLife,
		secret:           secret,
		signingMethod:    jwt.SigningMethodHS256,
	}
}

type token struct {
	str     string
	userID  scalar.ID
	expire  time.Time
	issueAt time.Time
	jti     string
}

func (t token) IssueAt() time.Time {
	return t.issueAt
}

func (t token) Expire() time.Time {
	return t.expire
}

func (t token) String() string {
	return t.str
}

func (t token) UserID() scalar.ID {
	return t.userID
}

func (t token) JTI() string {
	return t.jti
}

func (ts *TokenSource) formatError(err error) error {
	if err == nil {
		return err
	}
	if errors.Is(err, jwt.ErrTokenExpired) {
		return apperror.New(
			"INVALID_TOKEN",
			"token is expired, please login again",
			"令牌已过期，请重新登录",
		)
	}
	if errors.Is(err, jwt.ErrTokenMalformed) {
		return apperror.New(
			"INVALID_TOKEN",
			"token is malformed",
			"令牌格式有误",
		)
	}
	if errors.Is(err, jwt.ErrTokenSignatureInvalid) {
		return apperror.New(
			"INVALID_TOKEN",
			"token signature invalid",
			"令牌签名不匹配",
		)
	}
	return err
}

// NewAccessToken 签发短效访问令牌，仅包含标准的 JWT 注册声明
func (ts *TokenSource) NewAccessToken(ctx context.Context, deviceID scalar.ID) (device.Token, error) {
	var now = time.Now()
	var expire = now.Add(ts.accessTokenLife)
	var jti = uuid.NewString()
	var t = jwt.NewWithClaims(
		ts.signingMethod,
		jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expire),
			IssuedAt:  jwt.NewNumericDate(now),
			Subject:   deviceID.String(),
			ID:        jti,
		},
	)
	str, err := t.SignedString(ts.secret)
	if err != nil {
		return nil, ts.formatError(err)
	}

	return token{
		str:     str,
		userID:  deviceID,
		expire:  expire,
		issueAt: now,
		jti:     jti,
	}, nil
}

// NewRefreshToken 签发长效刷新令牌，包含 JTI 用于吊销检查
func (ts *TokenSource) NewRefreshToken(ctx context.Context, deviceID scalar.ID) (device.Token, error) {
	var now = time.Now()
	var expire = now.Add(ts.refreshTokenLife)
	var jti = uuid.NewString()
	var t = jwt.NewWithClaims(
		ts.signingMethod,
		jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expire),
			IssuedAt:  jwt.NewNumericDate(now),
			Subject:   deviceID.String(),
			ID:        jti,
		},
	)
	str, err := t.SignedString(ts.secret)
	if err != nil {
		return nil, ts.formatError(err)
	}

	return token{
		str:     str,
		userID:  deviceID,
		expire:  expire,
		issueAt: now,
		jti:     jti,
	}, nil
}

// VerifyAccessToken 校验访问令牌，仅验证签名与有效期，不检查设备状态
func (ts *TokenSource) VerifyAccessToken(ctx context.Context, rawToken string) (device.Token, error) {
	var claims jwt.RegisteredClaims
	_, err := jwt.ParseWithClaims(
		rawToken,
		&claims,
		func(t *jwt.Token) (interface{}, error) {
			if t.Method.Alg() != ts.signingMethod.Alg() {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Method.Alg())
			}
			return ts.secret, nil
		},
	)
	if err != nil {
		return nil, ts.formatError(err)
	}
	deviceID, err := scalar.ParseID(claims.Subject)
	if err != nil {
		return nil, err
	}
	return token{
		str:     rawToken,
		userID:  deviceID,
		expire:  claims.ExpiresAt.Time,
		issueAt: claims.IssuedAt.Time,
		jti:     claims.ID,
	}, nil
}

// VerifyRefreshToken 校验刷新令牌：验证签名、有效期，并检查吊销列表
func (ts *TokenSource) VerifyRefreshToken(ctx context.Context, rawToken string) (device.Token, error) {
	var claims jwt.RegisteredClaims
	_, err := jwt.ParseWithClaims(
		rawToken,
		&claims,
		func(t *jwt.Token) (interface{}, error) {
			if t.Method.Alg() != ts.signingMethod.Alg() {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Method.Alg())
			}
			return ts.secret, nil
		},
	)
	if err != nil {
		return nil, ts.formatError(err)
	}

	deviceID, err := scalar.ParseID(claims.Subject)
	if err != nil {
		return nil, err
	}

	return token{
		str:     rawToken,
		userID:  deviceID,
		expire:  claims.ExpiresAt.Time,
		issueAt: claims.IssuedAt.Time,
		jti:     claims.ID,
	}, nil
}
