package tokenrw

import "context"

type Transfer int

const (
	InlineTransfer Transfer = iota
	CookieTransfer
)

type contextKeyTransfer struct{}

func WithTransfer(ctx context.Context, v Transfer) context.Context {
	return context.WithValue(ctx, contextKeyTransfer{}, v)
}

func ContextTransfer(ctx context.Context) Transfer {
	var v, _ = ctx.Value(contextKeyTransfer{}).(Transfer)
	return v
}
