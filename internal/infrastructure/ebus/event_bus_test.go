package ebus

import (
	"context"
	"testing"
	"time"

	"main/internal/pubsub"
	"main/internal/scalar"
	"main/internal/shared"

	"github.com/stretchr/testify/assert"
)

func TestNewEventBus(t *testing.T) {
	topic, _ := pubsub.NewInMemoryTopic[*shared.SessionDTO]()
	bus := NewEventBus(topic)

	assert.NotNil(t, bus)
	assert.NotNil(t, bus.Session)
}

func TestPublishSession(t *testing.T) {
	topic, _ := pubsub.NewInMemoryTopic[*shared.SessionDTO]()
	bus := NewEventBus(topic)

	dto := &shared.SessionDTO{
		ID:        scalar.ToID("test-id"),
		Directory: "test-dir",
		Stats: &shared.StatsDTO{
			Total:       10,
			Processed:   0,
			Kept:        0,
			Reviewed:    0,
			Rejected:    0,
			Remaining:   10,
			IsCompleted: false,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	ctx := context.Background()
	bus.PublishSession(ctx, dto)
}

func TestSubscribeSession(t *testing.T) {
	topic, _ := pubsub.NewInMemoryTopic[*shared.SessionDTO]()
	bus := NewEventBus(topic)

	dto := &shared.SessionDTO{
		ID:        scalar.ToID("test-id"),
		Directory: "test-dir",
		Stats: &shared.StatsDTO{
			Total:       10,
			Processed:   0,
			Kept:        0,
			Reviewed:    0,
			Rejected:    0,
			Remaining:   10,
			IsCompleted: false,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	go func() {
		time.Sleep(100 * time.Millisecond)
		bus.PublishSession(ctx, dto)
	}()

	received := false
	for dto, err := range bus.SubscribeSession(ctx) {
		if err != nil {
			continue
		}
		if dto.ID == scalar.ToID("test-id") {
			received = true
			break
		}
	}

	assert.True(t, received, "Should receive published session")
}
