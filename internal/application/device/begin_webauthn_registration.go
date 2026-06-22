package device

import (
	"context"

	"github.com/go-webauthn/webauthn/protocol"
)

func (h *Handler) BeginWebAuthnRegistration(ctx context.Context) (*protocol.CredentialCreation, string, error) {
	return h.service.BeginRegistration(ctx)
}