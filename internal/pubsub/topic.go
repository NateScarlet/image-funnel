package pubsub

import (
	"context"
	"iter"
)

type Topic[T any] interface {
	// Publish event to every subscriber.
	Publish(ctx context.Context, v T, options ...PublishOption) error
	// Subscribe event sequence.
	Subscribe(ctx context.Context) iter.Seq2[T, error]
}

type PublishOptions struct {
}

func NewPublishOptions(options ...PublishOption) *PublishOptions {
	var opts = new(PublishOptions)
	for _, i := range options {
		i(opts)
	}
	return opts
}

type PublishOption func(opts *PublishOptions)
