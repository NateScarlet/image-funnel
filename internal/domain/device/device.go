package device

import (
	"time"

	"main/internal/scalar"
)

// Device 表示一个受信任的 WebAuthn 设备
type Device struct {
	id                   scalar.ID
	credentialID         []byte
	publicKey            []byte
	signCount            uint32
	createdAt            time.Time
	lastLoginAt          time.Time
	lastLoginIP          string
	userAgent            string
	refreshTokenID       string    // 当前刷新令牌的 JTI
	refreshTokenExpiresAt time.Time // 当前刷新令牌的过期时间
}

func (d *Device) ID() scalar.ID {
	return d.id
}

func (d *Device) CredentialID() []byte {
	return d.credentialID
}

func (d *Device) PublicKey() []byte {
	return d.publicKey
}

func (d *Device) SignCount() uint32 {
	return d.signCount
}

func (d *Device) CreatedAt() time.Time {
	return d.createdAt
}

func (d *Device) LastLoginAt() time.Time {
	return d.lastLoginAt
}

func (d *Device) LastLoginIP() string {
	return d.lastLoginIP
}

func (d *Device) UserAgent() string {
	return d.userAgent
}

// UpdateLogin 记录最后登录时间、IP 和 User-Agent
func (d *Device) UpdateLogin(ip string, ua string, at time.Time) {
	d.lastLoginIP = ip
	if ua != "" {
		d.userAgent = ua
	}
	d.lastLoginAt = at
}

// UpdateSignCount 更新签名计数
func (d *Device) UpdateSignCount(count uint32) {
	d.signCount = count
}

// RefreshTokenID 返回当前刷新令牌的 JTI，空字符串表示无活跃刷新令牌
func (d *Device) RefreshTokenID() string {
	return d.refreshTokenID
}

// RefreshTokenExpiresAt 返回当前刷新令牌的过期时间
func (d *Device) RefreshTokenExpiresAt() time.Time {
	return d.refreshTokenExpiresAt
}

// UpdateRefreshToken 更新设备关联的刷新令牌信息
func (d *Device) UpdateRefreshToken(jti string, expiresAt time.Time) {
	d.refreshTokenID = jti
	d.refreshTokenExpiresAt = expiresAt
}

func FromRepository(
	id scalar.ID,
	credentialID []byte,
	publicKey []byte,
	signCount uint32,
	createdAt time.Time,
	lastLoginAt time.Time,
	lastLoginIP string,
	userAgent string,
	refreshTokenID string,
	refreshTokenExpiresAt time.Time,
) *Device {
	return &Device{
		id:                   id,
		credentialID:         credentialID,
		publicKey:            publicKey,
		signCount:            signCount,
		createdAt:            createdAt,
		lastLoginAt:          lastLoginAt,
		lastLoginIP:          lastLoginIP,
		userAgent:            userAgent,
		refreshTokenID:       refreshTokenID,
		refreshTokenExpiresAt: refreshTokenExpiresAt,
	}
}