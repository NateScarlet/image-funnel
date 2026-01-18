package ebus

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"main/internal/application/session"
	"main/internal/pubsub"
)

func TestNewEventBus(t *testing.T) {
	topic, _ := pubsub.NewInMemoryTopic[*session.SessionDTO]()
	bus := NewEventBus(topic)

	assert.NotNil(t, bus)
	assert.NotNil(t, bus.Session)
}

func TestPublishSession(t *testing.T) {
	topic, _ := pubsub.NewInMemoryTopic[*session.SessionDTO]()
	bus := NewEventBus(topic)

	dto := &session.SessionDTO{
		ID:        "test-id",
		Directory: "test-dir",
		Status:     session.StatusActive,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	ctx := context.Background()
	bus.PublishSession(ctx, dto)
}

func TestSubscribeSession(t *testing.T) {
	topic, _ := pubsub.NewInMemoryTopic[*session.SessionDTO]()
	bus := NewEventBus(topic)

	dto := &session.SessionDTO{
		ID:        "test-id",
		Directory: "test-dir",
		Status:     session.StatusActive,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
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
		if dto.ID == "test-id" {
			received = true
			break
		}
	}

	assert.True(t, received, "Should receive published session")
}
