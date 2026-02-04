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

func TestSession_KeptImages(t *testing.T) {
	// Create test images with mixed names to test sorting
	images := []*image.Image{
		image.NewImage(scalar.ToID("1"), "b.jpg", "/path/b.jpg", 100, time.Now(), nil, 100, 100),
		image.NewImage(scalar.ToID("2"), "a.jpg", "/path/a.jpg", 200, time.Now(), nil, 100, 100),
		image.NewImage(scalar.ToID("3"), "c.jpg", "/path/c.jpg", 300, time.Now(), nil, 100, 100),
		image.NewImage(scalar.ToID("4"), "d.jpg", "/path/d.jpg", 400, time.Now(), nil, 100, 100),
	}

	session := NewSession(scalar.ToID("s1"), scalar.ToID("d1"), nil, 10, images)

	// 1. Initial State: No kept images
	kept := session.KeptImages(10, 0)
	assert.Empty(t, kept, "Should be empty initially")

	// 2. Mark images with different actions
	// Keep a.jpg and c.jpg
	require.NoError(t, session.MarkImage(scalar.ToID("2"), shared.ImageActionKeep)) // a.jpg
	require.NoError(t, session.MarkImage(scalar.ToID("3"), shared.ImageActionKeep)) // c.jpg
	// Reject b.jpg
	require.NoError(t, session.MarkImage(scalar.ToID("1"), shared.ImageActionReject)) // b.jpg
	// d.jpg is Shelved

	// 3. Verify KeptImages returns only kept images, sorted by filename
	kept = session.KeptImages(10, 0)
	require.Len(t, kept, 2)
	assert.Equal(t, "a.jpg", kept[0].Filename())
	assert.Equal(t, "c.jpg", kept[1].Filename())

	// 4. Test Pagination
	// Limit 1
	kept = session.KeptImages(1, 0)
	require.Len(t, kept, 1)
	assert.Equal(t, "a.jpg", kept[0].Filename())

	// Offset 1
	kept = session.KeptImages(10, 1)
	require.Len(t, kept, 1)
	assert.Equal(t, "c.jpg", kept[0].Filename())

	// Limit 1, Offset 1
	kept = session.KeptImages(1, 1)
	require.Len(t, kept, 1)
	assert.Equal(t, "c.jpg", kept[0].Filename())

	// Out of bounds
	kept = session.KeptImages(10, 5)
	assert.Empty(t, kept)
}
