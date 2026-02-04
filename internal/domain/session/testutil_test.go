package session

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"main/internal/domain/image"
	"main/internal/domain/metadata"
	"main/internal/scalar"
	"main/internal/shared"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// #region Helper Functions

// ActionOf returns the actual status of an image in the session (zero value = Pending/Unprocessed).
func ActionOf(s *Session, id scalar.ID) shared.ImageAction {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.actions[id]
}

// ImagesOf returns all images currently tracked by the session.
func ImagesOf(s *Session) []*image.Image {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Collect(maps.Values(s.images))
}

// createTestImages creates a slice of dummy images for testing.
func createTestImages(count int) []*image.Image {
	images := make([]*image.Image, count)
	for i := 0; i < count; i++ {
		images[i] = image.NewImage(
			scalar.ToID(fmt.Sprintf("img-%d", i)),
			"test.jpg",
			fmt.Sprintf("/test/test-%d.jpg", i),
			1000,
			time.Now(),
			nil,
			1920,
			1080,
		)
	}
	return images
}

// createTestImagesWithRatings creates a slice of images with specified ratings.
func createTestImagesWithRatings(ratings []int) []*image.Image {
	images := make([]*image.Image, len(ratings))
	for i, rating := range ratings {
		xmpData := metadata.NewXMPData(rating, "", time.Time{})
		images[i] = image.NewImage(
			scalar.ToID(fmt.Sprintf("img-%d", i)),
			"test.jpg",
			fmt.Sprintf("/test/test-%d.jpg", i),
			1000,
			time.Now(),
			xmpData,
			1920,
			1080,
		)
	}
	return images
}

// setupTestSession creates and returns a test session object.
func setupTestSession(_ *testing.T, imageCount int, targetKeep int) *Session {
	filter := &shared.ImageFilters{Rating: []int{0}}
	images := createTestImages(imageCount)
	session := NewSession(scalar.ToID("test-id"), scalar.ToID("test-dir-id"), filter, targetKeep, images)
	return session
}

// markImagesInSession marks images in the session queue using the provided action function.
func markImagesInSession(t *testing.T, session *Session, actionFn func(index int) shared.ImageAction) {
	for i := 0; i < len(session.queue); i++ {
		action := actionFn(i)
		err := session.MarkImage(session.queue[i].ID(), action)
		require.NoError(t, err)
	}
}

// #endregion

// #region Fakes

// FakeMetadataRepo is a mock implementation of MetadataRepository.
type FakeMetadataRepo struct {
	Data map[string]*metadata.XMPData
}

func NewFakeMetadataRepo() *FakeMetadataRepo {
	return &FakeMetadataRepo{
		Data: make(map[string]*metadata.XMPData),
	}
}

func (f *FakeMetadataRepo) Write(path string, data *metadata.XMPData) error {
	f.Data[path] = data
	return nil
}

func (f *FakeMetadataRepo) Read(path string) (*metadata.XMPData, error) {
	if d, ok := f.Data[path]; ok {
		return d, nil
	}
	return nil, nil
}

// FakeSessionRepo is a mock implementation of Repository.
type FakeSessionRepo struct {
	Sessions map[scalar.ID]*Session
}

func NewFakeSessionRepo() *FakeSessionRepo {
	return &FakeSessionRepo{
		Sessions: make(map[scalar.ID]*Session),
	}
}

func (f *FakeSessionRepo) Save(s *Session) error {
	f.Sessions[s.ID()] = s
	return nil
}

func (f *FakeSessionRepo) Get(id scalar.ID) (*Session, error) {
	if s, ok := f.Sessions[id]; ok {
		return s, nil
	}
	return nil, errors.New("not found")
}

func (f *FakeSessionRepo) FindByDirectory(directoryID scalar.ID) iter.Seq2[*Session, error] {
	return func(yield func(*Session, error) bool) {
		for _, s := range f.Sessions {
			if s.DirectoryID() == directoryID {
				if !yield(s, nil) {
					return
				}
			}
		}
	}
}

// FakeEventBus is a mock implementation of EventBus.
type FakeEventBus struct{}

func (f *FakeEventBus) SubscribeFileChanged(ctx context.Context) iter.Seq2[*shared.FileChangedEvent, error] {
	return func(yield func(*shared.FileChangedEvent, error) bool) {}
}

// #endregion
