package device

import (
	"github.com/go-webauthn/webauthn/webauthn"
)

// webauthnUser 实现了 webauthn.User 接口
type webauthnUser struct {
	id          []byte
	name        string
	displayName string
	credentials []webauthn.Credential
}

// WebAuthnID returns the user's ID
func (u *webauthnUser) WebAuthnID() []byte {
	return u.id
}

// WebAuthnName returns the user's name
func (u *webauthnUser) WebAuthnName() string {
	return u.name
}

// WebAuthnDisplayName returns the user's display name
func (u *webauthnUser) WebAuthnDisplayName() string {
	return u.displayName
}

// WebAuthnIcon is not (yet) implemented
func (u *webauthnUser) WebAuthnIcon() string {
	return ""
}

// WebAuthnCredentials returns credentials owned by the user
func (u *webauthnUser) WebAuthnCredentials() []webauthn.Credential {
	return u.credentials
}
