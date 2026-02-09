package session

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"main/internal/domain/directory"
	"main/internal/domain/image"
	"main/internal/domain/metadata"
	"main/internal/scalar"
	"main/internal/shared"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// #region Helper Functions

// ActionOf returns the actual status of an image in the session (zero value = Pending/Unprocessed).
func ActionOf(s *Session, id scalar.ID) shared.ImageAction {
	return s.actions[id]
}

// ImagesOf returns all images currently tracked by the session.
func ImagesOf(s *Session) []*image.Image {
	return slices.Clone(s.images)
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
		imgIdx := session.queue[i]
		imgID := session.images[imgIdx].ID()
		action := actionFn(i)
		err := session.MarkImage(imgID, action)
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

func (f *FakeSessionRepo) Create(s *Session) (func(), error) {
	f.Sessions[s.ID()] = s
	return func() {}, nil
}

func (f *FakeSessionRepo) Acquire(ctx context.Context, id scalar.ID) (*Session, func(), error) {
	if s, ok := f.Sessions[id]; ok {
		return s, func() {}, nil
	}
	return nil, nil, errors.New("not found")
}

func (f *FakeSessionRepo) Get(id scalar.ID) (*Session, error) {
	if s, ok := f.Sessions[id]; ok {
		return s, nil
	}
	return nil, errors.New("not found")
}

func (f *FakeSessionRepo) FindByDirectory(directoryID scalar.ID) iter.Seq2[scalar.ID, error] {
	return func(yield func(scalar.ID, error) bool) {
		for _, s := range f.Sessions {
			if s.DirectoryID() == directoryID {
				if !yield(s.ID(), nil) {
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

// FakeScanner is a mock implementation of directory.Scanner
type FakeScanner struct {
	MetaRepo *FakeMetadataRepo
	BaseDir  string
	Images   map[string]*image.Image // RelPath -> Image
}

func (s *FakeScanner) Scan(ctx context.Context, relPath string) iter.Seq2[*image.Image, error] {
	return func(yield func(*image.Image, error) bool) {}
}

func (s *FakeScanner) LookupImage(ctx context.Context, relPath string) (*image.Image, error) {
	// Normalize path separators if needed, but simplistic match for now
	if img, ok := s.Images[relPath]; ok {
		// Update XMP from MetaRepo if available to simulate disk state
		fullPath := filepath.Join(s.BaseDir, relPath)
		if xmp, _ := s.MetaRepo.Read(fullPath); xmp != nil {
			// Create a copy with updated XMP logic if needed,
			// but strictly speaking LookupImage just reads file.
			// If we want to simulate "disk has XMP", we should probably
			// reflect that.
			// However, for ID consistency, we must return an image that produces the SAME ID
			// unless the file content/modtime changed.
			// In our tests, we don't change modtime usually.

			// Currently image.ID() depends on ModTime.
			// If we want to return the same ID, we must use the same ModTime.
			return image.NewImage(
				img.ID(),
				img.Filename(),
				img.Path(),
				img.Size(),
				img.ModTime(),
				xmp,
				img.Width(),
				img.Height(),
			), nil
		}
		return img, nil
	}
	// If not found in our "mock filesystem", return error
	return nil, os.ErrNotExist
}

func (s *FakeScanner) ScanDirectories(ctx context.Context, relPath string) iter.Seq2[*directory.Directory, error] {
	return func(yield func(*directory.Directory, error) bool) {}
}

func (s *FakeScanner) AnalyzeDirectory(ctx context.Context, relPath string) (*directory.DirectoryStats, error) {
	return &directory.DirectoryStats{}, nil
}

// #endregion
