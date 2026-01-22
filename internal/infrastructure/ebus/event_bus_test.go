package ebus

import (
	"context"
	"testing"
	"time"

	appimage "main/internal/application/image"
	"main/internal/application/session"
	dsession "main/internal/domain/session"
	"main/internal/pubsub"

	"main/internal/scalar"
	"main/internal/shared"

	"github.com/stretchr/testify/assert"
)

type mockURLSigner struct{}

func (m *mockURLSigner) GenerateSignedURL(path string, opts ...appimage.SignOption) (string, error) {
	return "signed://" + path, nil
}

func TestNewEventBus(t *testing.T) {
	sessionTopic, _ := pubsub.NewInMemoryTopic[*dsession.Session]()
	fileChangedTopic, _ := pubsub.NewInMemoryTopic[*shared.FileChangedEvent]()
	urlSigner := &mockURLSigner{}
	factory := session.NewSessionDTOFactory(urlSigner)
	bus := NewEventBus(sessionTopic, fileChangedTopic, factory)

	assert.NotNil(t, bus)
}

func TestSubscribeSession(t *testing.T) {
	sessionTopic, _ := pubsub.NewInMemoryTopic[*dsession.Session]()
	fileChangedTopic, _ := pubsub.NewInMemoryTopic[*shared.FileChangedEvent]()
	urlSigner := &mockURLSigner{}
	factory := session.NewSessionDTOFactory(urlSigner)
	bus := NewEventBus(sessionTopic, fileChangedTopic, factory)

	sess := dsession.NewSession(
		scalar.ToID("test-id"),
		scalar.ToID("test-dir"),
		&shared.ImageFilters{},
		10,
		nil,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	go func() {
		time.Sleep(100 * time.Millisecond)
		sessionTopic.Publish(ctx, sess)
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
	sessionTopic, _ := pubsub.NewInMemoryTopic[*dsession.Session]()
	fileChangedTopic, _ := pubsub.NewInMemoryTopic[*shared.FileChangedEvent]()
	urlSigner := &mockURLSigner{}
	factory := session.NewSessionDTOFactory(urlSigner)
	bus := NewEventBus(sessionTopic, fileChangedTopic, factory)

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
