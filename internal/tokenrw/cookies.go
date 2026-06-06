package tokenrw

import (
	"context"
	"net/http"
)

type Cookies interface {
	Get(name string) (string, error)
	Set(name, value string, maxAge int, path, domain string, secure, httpOnly bool, sameSite http.SameSite)
}

type contextKeyCookies struct{}

func WithCookies(ctx context.Context, v Cookies) context.Context {
	return context.WithValue(ctx, contextKeyCookies{}, v)
}

func ContextCookies(ctx context.Context) Cookies {
	var ret, _ = ctx.Value(contextKeyCookies{}).(Cookies)
	return ret
}
