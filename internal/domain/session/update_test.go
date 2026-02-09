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

func TestSession_MarkedButNotWritten_AfterNextRound(t *testing.T) {
	imgA := image.NewImage(
		scalar.ToID("img-a"),
		"test.jpg",
		"/test/test-a.jpg",
		1000,
		time.Now(),
		nil,
		1920,
		1080,
	)

	filter := &shared.ImageFilters{}
	session := NewSession(scalar.ToID("s1"), scalar.ToID("d1"), filter, 10, []*image.Image{imgA})

	assert.Equal(t, 1, len(ImagesOf(session)))

	session.RemoveImageByPath(imgA.Path())
	assert.Equal(t, 0, session.CurrentSize())

	imgAFresh := image.NewImage(
		scalar.ToID("img-a"),
		"test.jpg",
		"/test/test-a.jpg",
		1000,
		time.Now(),
		nil,
		1920,
		1080,
	)

	err := session.NextRound(filter, []*image.Image{imgAFresh})
	require.NoError(t, err)

	assert.Equal(t, 1, len(session.queue))
	assert.Equal(t, 1, len(ImagesOf(session)))

	err = session.MarkImage(imgAFresh.ID(), shared.ImageActionKeep)
	require.NoError(t, err)

	count := 0
	for range session.Actions() {
		count++
	}
	assert.Equal(t, 1, count)
}
