package device

import (
	"errors"
	"time"

	"main/internal/scalar"
)

type Factory struct{}

func NewFactory() *Factory {
	return &Factory{}
}

// New 创建一个新的 Device 实体，会进行参数校验
func (f *Factory) New(
	id scalar.ID,
	credentialID []byte,
	publicKey []byte,
	signCount uint32,
	lastLoginAt time.Time,
	lastLoginIP string,
	userAgent string,
) (*Device, error) {
	if id.IsZero() {
		return nil, errors.New("id is required")
	}
	if len(credentialID) == 0 {
		return nil, errors.New("credentialID is required")
	}
	if len(publicKey) == 0 {
		return nil, errors.New("publicKey is required")
	}

	return &Device{
		id:                    id,
		credentialID:          credentialID,
		publicKey:             publicKey,
		signCount:             signCount,
		createdAt:             time.Now(),
		lastLoginAt:           lastLoginAt,
		lastLoginIP:           lastLoginIP,
		userAgent:             userAgent,
		refreshTokenID:        "",
		refreshTokenExpiresAt: time.Time{},
	}, nil
}
