package ebus

import (
	"context"
	"testing"
	"time"

	"main/internal/application/session"
	"main/internal/domain/image"
	dsession "main/internal/domain/session"
	"main/internal/infrastructure/inmem"
	"main/internal/pubsub"

	"main/internal/scalar"
	"main/internal/shared"

	"github.com/stretchr/testify/assert"
)

func TestNewEventBus(t *testing.T) {
	sessionTopic, _ := pubsub.NewInMemoryTopic[scalar.ID]()
	fileChangedTopic, _ := pubsub.NewInMemoryTopic[*shared.FileChangedEvent]()
	factory := session.NewDTOFactory()
	sessionRepo := inmem.NewSessionRepository()
	bus := NewEventBus(sessionTopic, fileChangedTopic, sessionRepo, factory)

	assert.NotNil(t, bus)
}

func TestSubscribeSession(t *testing.T) {
	sessionTopic, _ := pubsub.NewInMemoryTopic[scalar.ID]()
	fileChangedTopic, _ := pubsub.NewInMemoryTopic[*shared.FileChangedEvent]()
	factory := session.NewDTOFactory()
	sessionRepo := inmem.NewSessionRepository()
	bus := NewEventBus(sessionTopic, fileChangedTopic, sessionRepo, factory)

	sess := dsession.New(
		scalar.ToID("test-id"),
		scalar.ToID("test-dir"),
		&shared.ImageFilters{},
		10,
		nil,
		image.NewFilterBuilder(),
	)
	// 先注册到 repo，再发布 ID
	release, err := sessionRepo.Create(sess)
	if err != nil {
		t.Fatal(err)
	}
	release()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	go func() {
		time.Sleep(100 * time.Millisecond)
		sessionTopic.Publish(ctx, scalar.ToID("test-id"))
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

func TestFileChanged(t *testing.T) {
	sessionTopic, _ := pubsub.NewInMemoryTopic[scalar.ID]()
	fileChangedTopic, _ := pubsub.NewInMemoryTopic[*shared.FileChangedEvent]()
	factory := session.NewDTOFactory()
	sessionRepo := inmem.NewSessionRepository()
	bus := NewEventBus(sessionTopic, fileChangedTopic, sessionRepo, factory)

	event := &shared.FileChangedEvent{
		DirectoryID: scalar.ToID("test-dir"),
		RelPath:     "test.jpg",
		Action:      shared.FileActionCreate,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	go func() {
		time.Sleep(100 * time.Millisecond)
		bus.PublishFileChanged(ctx, event)
	}()

	received := false
	for e, err := range bus.SubscribeFileChanged(ctx) {
		if err != nil {
			continue
		}
		if e.RelPath == "test.jpg" {
			received = true
			break
		}
	}

	assert.True(t, received, "Should receive published file changed event")
}
