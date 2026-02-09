package session

import (
	"main/internal/domain/image"
	"main/internal/scalar"
	"main/internal/shared"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSession_ShouldInitializeCorrectly(t *testing.T) {
	filter := &shared.ImageFilters{Rating: []int{0, 1, 2}}
	images := createTestImages(10)

	session := NewSession(scalar.ToID("test-id"), scalar.ToID("test-dir-id"), filter, 5, images)

	assert.NotEmpty(t, session.ID(), "Session ID should not be empty")
	assert.Equal(t, scalar.ToID("test-dir-id"), session.DirectoryID(), "DirectoryID should match")
	assert.Equal(t, filter, session.Filter(), "Filter should match")
	assert.Equal(t, 5, session.TargetKeep(), "TargetKeep should match")
	assert.False(t, session.Stats().IsCompleted, "IsCompleted should be false initially")
	assert.Equal(t, 10, len(ImagesOf(session)), "Images count should match")
	assert.Equal(t, 10, len(session.queue), "Queue count should match")
	assert.Equal(t, 0, session.CurrentIndex(), "CurrentIndex should be 0")
	assert.False(t, session.CanUndo(), "CanUndo should be false initially")
}

func TestCurrentImage_ShouldReturnCorrectImage(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	currentImage := session.CurrentImage()
	assert.NotNil(t, currentImage, "CurrentImage should not be nil")
	assert.Equal(t, session.images[session.queue[0]].ID(), currentImage.ID(), "CurrentImage ID should match")

	firstImageID := session.images[session.queue[0]].ID()
	err := session.MarkImage(firstImageID, shared.ImageActionKeep)
	require.NoError(t, err)

	currentImage = session.CurrentImage()
	assert.NotNil(t, currentImage, "CurrentImage should not be nil")
	assert.NotEqual(t, firstImageID, currentImage.ID(), "CurrentImage ID should not match first image")
	assert.Equal(t, session.images[session.queue[1]].ID(), currentImage.ID(), "CurrentImage ID should match second image")
}

func TestSession_KeptImages(t *testing.T) {
	images := []*image.Image{
		image.NewImage(scalar.ToID("1"), "b.jpg", "/path/b.jpg", 100, time.Now(), nil, 100, 100),
		image.NewImage(scalar.ToID("2"), "a.jpg", "/path/a.jpg", 200, time.Now(), nil, 100, 100),
		image.NewImage(scalar.ToID("3"), "c.jpg", "/path/c.jpg", 300, time.Now(), nil, 100, 100),
		image.NewImage(scalar.ToID("4"), "d.jpg", "/path/d.jpg", 400, time.Now(), nil, 100, 100),
	}

	session := NewSession(scalar.ToID("s1"), scalar.ToID("d1"), nil, 10, images)

	kept := session.KeptImages(10, 0)
	assert.Empty(t, kept, "Should be empty initially")

	require.NoError(t, session.MarkImage(scalar.ToID("2"), shared.ImageActionKeep))   // a.jpg
	require.NoError(t, session.MarkImage(scalar.ToID("3"), shared.ImageActionKeep))   // c.jpg
	require.NoError(t, session.MarkImage(scalar.ToID("1"), shared.ImageActionReject)) // b.jpg

	kept = session.KeptImages(10, 0)
	require.Len(t, kept, 2)
	assert.Equal(t, "a.jpg", kept[0].Filename())
	assert.Equal(t, "c.jpg", kept[1].Filename())

	kept = session.KeptImages(1, 0)
	require.Len(t, kept, 1)
	assert.Equal(t, "a.jpg", kept[0].Filename())

	kept = session.KeptImages(10, 1)
	require.Len(t, kept, 1)
	assert.Equal(t, "c.jpg", kept[1-1].Filename())

	kept = session.KeptImages(1, 1)
	require.Len(t, kept, 1)
	assert.Equal(t, "c.jpg", kept[0].Filename())

	kept = session.KeptImages(10, 5)
	assert.Empty(t, kept)
}
