package tokenrw

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	CookieNamePrefix  = "__Host-Token-"
	CookieTokenScheme = "cookie:"
	CookieTokenPrefix = CookieTokenScheme + CookieNamePrefix
)

var (
	ErrCookiesUnavailable = fmt.Errorf("tokenrw: cookies unavailable")
	ErrNoCookie           = http.ErrNoCookie
)

func Read(ctx context.Context, token string) (rawToken string, forgetToken func(), err error) {
	if !strings.HasPrefix(token, CookieTokenPrefix) {
		return token, func() {}, nil
	}
	var cookies = ContextCookies(ctx)
	if cookies == nil {
		return "", func() {}, ErrCookiesUnavailable
	}
	var name = token[len(CookieTokenScheme):]
	rawToken, err = cookies.Get(name)
	if err != nil {
		return "", func() {}, fmt.Errorf("tokenrw: failed to get cookie %q: %w", name, err)
	}
	if rawToken == "" {
		return "", func() {}, fmt.Errorf("tokenrw: cookie %q is not provided", name)
	}
	return rawToken, func() {
		cookies.Set(name, "", -1, "/", "", true, true, http.SameSiteStrictMode)
	}, nil
}

func Write(ctx context.Context, rawToken string, expiresAt time.Time) (string, error) {
	switch transfer := ContextTransfer(ctx); transfer {
	case InlineTransfer:
		return rawToken, nil
	case CookieTransfer:
		var cookies = ContextCookies(ctx)
		if cookies == nil {
			return "", ErrCookiesUnavailable
		}
		var name = CookieNamePrefix + uuid.NewString()
		var maxAge = int(time.Until(expiresAt).Seconds()) + 1
		if maxAge < 0 {
			return "", fmt.Errorf("tokenrw: token already expired")
		}
		cookies.Set(name, rawToken, maxAge, "/", "", true, true, http.SameSiteStrictMode)
		return CookieTokenScheme + name, nil
	default:
		return "", fmt.Errorf("tokenrw: unknown transfer %q", transfer)
	}
}
