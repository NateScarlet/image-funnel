package inmem

import (
	"testing"
	"time"

	"main/internal/domain/session"
	"main/internal/scalar"

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

	img := session.NewImage(scalar.ToID("test-id"), "test.jpg", "/test/test.jpg", 1000, time.Now(), 0, false)
	sess := session.NewSession("test-dir", session.NewImageFilters(nil), 5, []*session.Image{img})
	err := repo.Save(sess)
	require.NoError(t, err)

	_, err = repo.FindByID(sess.ID())
	require.NoError(t, err)
}

func TestFindByID(t *testing.T) {
	repo := NewSessionRepository()

	img := session.NewImage(scalar.ToID("test-id"), "test.jpg", "/test/test.jpg", 1000, time.Now(), 0, false)
	sess := session.NewSession("test-dir", session.NewImageFilters(nil), 5, []*session.Image{img})
	err := repo.Save(sess)
	require.NoError(t, err)

	found, err := repo.FindByID(sess.ID())
	require.NoError(t, err)
	assert.Equal(t, sess.ID(), found.ID())
}

func TestFindByID_NotFound(t *testing.T) {
	repo := NewSessionRepository()

	_, err := repo.FindByID(scalar.ToID("non-existent-id"))
	require.Error(t, err)
}

func TestFindAll(t *testing.T) {
	repo := NewSessionRepository()

	img1 := session.NewImage(scalar.ToID("test-id-1"), "test1.jpg", "/test/test1.jpg", 1000, time.Now(), 0, false)
	sess1 := session.NewSession("test-dir-1", session.NewImageFilters(nil), 5, []*session.Image{img1})

	img2 := session.NewImage(scalar.ToID("test-id-2"), "test2.jpg", "/test/test2.jpg", 1000, time.Now(), 0, false)
	sess2 := session.NewSession("test-dir-2", session.NewImageFilters(nil), 5, []*session.Image{img2})

	err := repo.Save(sess1)
	require.NoError(t, err)
	err = repo.Save(sess2)
	require.NoError(t, err)

	all, err := repo.FindAll()
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestDelete(t *testing.T) {
	repo := NewSessionRepository()

	img := session.NewImage(scalar.ToID("test-id"), "test.jpg", "/test/test.jpg", 1000, time.Now(), 0, false)
	sess := session.NewSession("test-dir", session.NewImageFilters(nil), 5, []*session.Image{img})
	err := repo.Save(sess)
	require.NoError(t, err)

	err = repo.Delete(sess.ID())
	require.NoError(t, err)

	_, err = repo.FindByID(sess.ID())
	require.Error(t, err)
}
