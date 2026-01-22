package graphql

import (
	"context"
	"errors"
	"iter"

	"github.com/99designs/gqlgen/graphql/handler/transport"
)

func SubscriptionError(ctx context.Context, err error) {
	transport.AddSubscriptionError(ctx, ErrorPresenter(ctx, err))
}

func SubscriptionFromSeq[T any](ctx context.Context, seq iter.Seq2[T, error]) (<-chan T, error) {
	var c = make(chan T)
	go func() {
		defer close(c)
		for dto, err := range seq {
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					SubscriptionError(ctx, err)
				}
				return
			}
			c <- dto
		}
	}()
	return c, nil
}
