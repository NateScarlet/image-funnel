package pubsub

import "fmt"

var (
	ErrTopicDisposed     = fmt.Errorf("pubsub: topic disposed")
	ErrUndeliveredEvents = fmt.Errorf("pubsub: undelivered events")
)
