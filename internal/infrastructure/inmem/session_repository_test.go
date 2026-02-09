package inmem

import (
	"context"
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
	release, err := repo.Create(sess)
	require.NoError(t, err)
	release() // 先释放 Create 的锁

	_, release2, err := repo.Acquire(context.Background(), sess.ID())
	require.NoError(t, err)
	release2()
}

func TestAcquire(t *testing.T) {
	repo := NewSessionRepository()

	img := image.NewImage(scalar.ToID("test-id"), "test.jpg", "/test/test.jpg", 1000, time.Now(), nil, 1920, 1080)
	sess := session.NewSession(scalar.ToID("test-id"), scalar.ToID("test-dir"), &shared.ImageFilters{Rating: nil}, 5, []*image.Image{img})
	release1, err := repo.Create(sess)
	require.NoError(t, err)
	release1() // 先释放 Create 的锁

	found, release, err := repo.Acquire(context.Background(), sess.ID())
	require.NoError(t, err)
	assert.Equal(t, sess.ID(), found.ID())
	release()
}

func TestAcquire_NotFound(t *testing.T) {
	repo := NewSessionRepository()

	_, _, err := repo.Acquire(context.Background(), scalar.ToID("non-existent-id"))
	require.Error(t, err)
}
