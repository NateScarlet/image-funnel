package device_test

import (
	"context"
	"testing"

	appdevice "main/internal/application/device"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthStatus_ClientIP(t *testing.T) {
	handler := &appdevice.Handler{}

	ctx := context.Background()
	ctx = appdevice.WithRemoteIP(ctx, "192.168.1.100")
	ctx = appdevice.WithTrustedIP(ctx, false)

	status, err := handler.AuthStatus(ctx)
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.100", status.ClientIP)
	assert.False(t, status.IsTrustedIP)
	assert.False(t, status.CanAccess)
}
