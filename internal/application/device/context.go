package device

import (
	"context"
	"main/internal/scalar"
)

type contextKey string

const (
	trustedDeviceKey contextKey = "trustedDevice"
	trustedIPKey     contextKey = "trustedIP"
	remoteIPKey      contextKey = "remoteIP"
	userAgentKey     contextKey = "userAgent"
	rawTokenKey      contextKey = "rawToken"
)

func WithTrustedDevice(ctx context.Context, deviceID scalar.ID) context.Context {
	return context.WithValue(ctx, trustedDeviceKey, deviceID)
}

func IsTrustedDevice(ctx context.Context) bool {
	_, ok := ctx.Value(trustedDeviceKey).(scalar.ID)
	return ok
}

func TrustedDeviceID(ctx context.Context) (scalar.ID, bool) {
	id, ok := ctx.Value(trustedDeviceKey).(scalar.ID)
	return id, ok
}

func WithTrustedIP(ctx context.Context, isTrusted bool) context.Context {
	return context.WithValue(ctx, trustedIPKey, isTrusted)
}

func IsTrustedIP(ctx context.Context) bool {
	v, ok := ctx.Value(trustedIPKey).(bool)
	return ok && v
}

func WithRemoteIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, remoteIPKey, ip)
}

func RemoteIP(ctx context.Context) string {
	v, ok := ctx.Value(remoteIPKey).(string)
	if !ok {
		return "unknown"
	}
	return v
}

func WithUserAgent(ctx context.Context, ua string) context.Context {
	return context.WithValue(ctx, userAgentKey, ua)
}

func UserAgent(ctx context.Context) string {
	v, ok := ctx.Value(userAgentKey).(string)
	if !ok {
		return ""
	}
	return v
}