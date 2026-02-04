package inmem

import (
	"testing"
	"time"

	"main/internal/domain/image"
	"main/internal/domain/session"
	"main/internal/scalar"
	"main/internal/shared"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSessionRepository(t *testing.T) {
	repo := NewSessionRepository()
	assert.NotNil(t, repo)
	assert.NotNil(t, repo.sessions)
}

func TestSave(t *testing.T) {
	repo := NewSessionRepository()

	img := image.NewImage(scalar.ToID("test-id"), "test.jpg", "/test/test.jpg", 1000, time.Now(), nil, 1920, 1080)
	sess := session.NewSession(scalar.ToID("test-id"), scalar.ToID("test-dir"), &shared.ImageFilters{Rating: nil}, 5, []*image.Image{img})
	err := repo.Save(sess)
	require.NoError(t, err)

	_, err = repo.Get(sess.ID())
	require.NoError(t, err)
}

func TestGet(t *testing.T) {
	repo := NewSessionRepository()

	img := image.NewImage(scalar.ToID("test-id"), "test.jpg", "/test/test.jpg", 1000, time.Now(), nil, 1920, 1080)
	sess := session.NewSession(scalar.ToID("test-id"), scalar.ToID("test-dir"), &shared.ImageFilters{Rating: nil}, 5, []*image.Image{img})
	err := repo.Save(sess)
	require.NoError(t, err)

	found, err := repo.Get(sess.ID())
	require.NoError(t, err)
	assert.Equal(t, sess.ID(), found.ID())
}

func TestGet_NotFound(t *testing.T) {
	repo := NewSessionRepository()

	_, err := repo.Get(scalar.ToID("non-existent-id"))
	require.Error(t, err)
}
