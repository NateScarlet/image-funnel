package pairing

import (
	"time"

	"main/internal/scalar"
)

// Request 表示一个尚未被批准的设备配对请求。
// 为避免与 device 领域的循环依赖，它不直接引用 device.Device 实体，
// 而是通过解构字段来存储配对设备的元数据与凭证信息。
type Request struct {
	code         string
	deviceID     scalar.ID
	credentialID []byte
	publicKey    []byte
	signCount    uint32
	createdAt    time.Time
	ip           string
	userAgent    string
}

// Code 返回配对请求的唯一验证码
func (r *Request) Code() string {
	return r.code
}

// DeviceID 返回该待配对设备的唯一标识
func (r *Request) DeviceID() scalar.ID {
	return r.deviceID
}

// CredentialID 返回 WebAuthn 的凭证 ID
func (r *Request) CredentialID() []byte {
	return r.credentialID
}

// PublicKey 返回 WebAuthn 凭证的公钥
func (r *Request) PublicKey() []byte {
	return r.publicKey
}

// SignCount 返回 WebAuthn 计数器的当前值，用于防重放与克隆检测
func (r *Request) SignCount() uint32 {
	return r.signCount
}

// CreatedAt 返回配对请求的创建时间
func (r *Request) CreatedAt() time.Time {
	return r.createdAt
}

// IP 返回配对请求的来源 IP 地址
func (r *Request) IP() string {
	return r.ip
}

// UserAgent 返回配对请求的来源 User-Agent
func (r *Request) UserAgent() string {
	return r.userAgent
}

// newPairingRequest 内部构造函数，不导出以强制外部通过 FromRepository 实例化
func newPairingRequest(
	code string,
	deviceID scalar.ID,
	credentialID []byte,
	publicKey []byte,
	signCount uint32,
	createdAt time.Time,
	ip string,
	userAgent string,
) *Request {
	return &Request{
		code:         code,
		deviceID:     deviceID,
		credentialID: credentialID,
		publicKey:    publicKey,
		signCount:    signCount,
		createdAt:    createdAt,
		ip:           ip,
		userAgent:    userAgent,
	}
}

// FromRepository 作为该实体的唯二外部构造入口，满足仓库重建及领域服务的受限创建需求
func FromRepository(
	code string,
	deviceID scalar.ID,
	credentialID []byte,
	publicKey []byte,
	signCount uint32,
	createdAt time.Time,
	ip string,
	userAgent string,
) *Request {
	return newPairingRequest(code, deviceID, credentialID, publicKey, signCount, createdAt, ip, userAgent)
}
