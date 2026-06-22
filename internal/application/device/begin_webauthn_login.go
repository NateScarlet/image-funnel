package device

import (
	"context"

	"github.com/go-webauthn/webauthn/protocol"
)

func (h *Handler) BeginWebAuthnLogin(ctx context.Context) (*protocol.CredentialAssertion, string, error) {
	return h.service.BeginLogin(ctx)
}