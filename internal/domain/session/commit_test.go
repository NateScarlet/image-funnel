package session

import (
	"context"
	"main/internal/domain/image"
	"main/internal/domain/metadata"
	"main/internal/pubsub"
	"main/internal/scalar"
	"main/internal/shared"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestCanCommit_InitialState_ShouldReturnFalse(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	assert.False(t, session.CanCommit(), "CanCommit should return false for initial state")
}

func TestCanCommit_AfterMarkingImages_ShouldReturnTrue(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	for i := 0; i < 3; i++ {
		err := session.MarkImage(session.images[session.queue[i]].ID(), shared.ImageActionKeep)
		require.NoError(t, err)
	}

	assert.True(t, session.CanCommit(), "CanCommit should return true after marking images")
}

func TestCanCommit_FirstRoundWithRejects_SecondRoundStart_ShouldBeAbleToCommit(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	markImagesInSession(t, session, func(index int) shared.ImageAction {
		action := shared.ImageActionKeep
		if index%2 == 0 {
			action = shared.ImageActionReject
		}
		return action
	})

	assert.True(t, session.Stats().IsCompleted, "Session should be completed when new queue length equals target")
	assert.True(t, session.CanCommit(), "CanCommit should return true after completing with kept images")

	stats := session.Stats()
	assert.Equal(t, 5, stats.Rejected, "Expected 5 rejected images")
}

func TestCanCommit_FirstRoundOnlyRejects_SecondRoundStart_ShouldBeAbleToCommit(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	markImagesInSession(t, session, func(index int) shared.ImageAction {
		return shared.ImageActionReject
	})

	assert.True(t, session.Stats().IsCompleted, "Session should be completed when all images are rejected")
	assert.True(t, session.CanCommit(), "CanCommit should return true after completing with rejected images")
}

func TestCanCommit_FirstRoundSingleReject_ShouldBeAbleToCommit(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	err := session.MarkImage(session.images[session.queue[0]].ID(), shared.ImageActionReject)
	require.NoError(t, err)

	assert.False(t, session.Stats().IsCompleted, "Session should not be completed")
	assert.True(t, session.CanCommit(), "CanCommit should return true after rejecting one image in first round")

	stats := session.Stats()
	assert.Equal(t, 1, stats.Rejected, "Expected 1 rejected image")
}

func TestActions_ShouldOnlyReturnMarkedImages(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	err := session.MarkImage(session.images[session.queue[0]].ID(), shared.ImageActionKeep)
	require.NoError(t, err)

	count := 0
	for range session.Actions() {
		count++
	}

	assert.Equal(t, 1, count, "Actions should only return explicitly marked images")

	found := false
	for _, action := range session.Actions() {
		if action == shared.ImageActionKeep {
			found = true
		}
	}
	assert.True(t, found, "Should contain the marked action")
}

func TestService_Commit_ShouldOnlyWriteMatchingImages(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "session_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	file1 := filepath.Join(tempDir, "test1.jpg")
	file2 := filepath.Join(tempDir, "test2.jpg")
	file3 := filepath.Join(tempDir, "test3.jpg")
	os.WriteFile(file1, []byte("fake"), 0644)
	os.WriteFile(file2, []byte("fake"), 0644)
	os.WriteFile(file3, []byte("fake"), 0644)

	fakeMeta := NewFakeMetadataRepo()
	fakeSessionRepo := NewFakeSessionRepo()
	fakeEventBus := &FakeEventBus{}
	topic, cleanup := pubsub.NewInMemoryTopic[*Session]()
	defer cleanup()

	fakeScanner := &FakeScanner{
		MetaRepo: fakeMeta,
		BaseDir:  tempDir,
		Images:   make(map[string]*image.Image),
	}

	svc, cleanupService := NewService(fakeSessionRepo, fakeMeta, fakeScanner, fakeEventBus, zap.NewNop(), topic, tempDir)
	defer cleanupService()

	filter := &shared.ImageFilters{Rating: []int{0}}

	img1 := image.NewImage(scalar.ToID("1"), "test1.jpg", file1, 100, time.Now(), metadata.NewXMPData(0, "", time.Time{}), 100, 100)
	img2 := image.NewImage(scalar.ToID("2"), "test2.jpg", file2, 100, time.Now(), metadata.NewXMPData(3, "", time.Time{}), 100, 100)
	img3 := image.NewImage(scalar.ToID("3"), "test3.jpg", file3, 100, time.Now(), metadata.NewXMPData(0, "", time.Time{}), 100, 100)

	fakeScanner.Images[filepath.Base(img1.Path())] = img1
	fakeScanner.Images[filepath.Base(img2.Path())] = img2
	fakeScanner.Images[filepath.Base(img3.Path())] = img3

	sess := NewSession(scalar.ToID("s1"), scalar.ToID("d1"), filter, 10, []*image.Image{img1, img3})

	sess.MarkImage(img1.ID(), shared.ImageActionKeep)

	sess.images = append(sess.images, img2)
	idx := len(sess.images) - 1
	sess.indexByID[img2.ID()] = idx
	sess.indexByPath[img2.Path()] = idx
	sess.actions[img2.ID()] = shared.ImageActionKeep

	sess.actions[img3.ID()] = shared.ImageActionReject

	writeActions := &shared.WriteActions{
		KeepRating:   5,
		ShelveRating: 0,
		RejectRating: -1,
	}

	success, errs := svc.Commit(context.Background(), sess, writeActions)

	require.Empty(t, errs)
	require.Equal(t, 2, success, "Should successfully write matching images")

	require.Contains(t, fakeMeta.Data, file1)
	require.Equal(t, 5, fakeMeta.Data[file1].Rating())
	require.Equal(t, shared.ImageActionKeep.String(), fakeMeta.Data[file1].Action())

	require.NotContains(t, fakeMeta.Data, file2)

	require.Contains(t, fakeMeta.Data, file3)
	require.Equal(t, -1, fakeMeta.Data[file3].Rating())
	require.Equal(t, shared.ImageActionReject.String(), fakeMeta.Data[file3].Action())
}

func TestService_Commit_UpdatesInMemoryState(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "session_test_memory")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	file1 := filepath.Join(tempDir, "test1.jpg")
	os.WriteFile(file1, []byte("fake"), 0644)

	fakeMeta := NewFakeMetadataRepo()
	fakeSessionRepo := NewFakeSessionRepo()
	fakeEventBus := &FakeEventBus{}
	topic, cleanup := pubsub.NewInMemoryTopic[*Session]()
	defer cleanup()

	fakeScanner := &FakeScanner{
		MetaRepo: fakeMeta,
		BaseDir:  tempDir,
		Images:   make(map[string]*image.Image),
	}

	svc, cleanupService := NewService(fakeSessionRepo, fakeMeta, fakeScanner, fakeEventBus, zap.NewNop(), topic, tempDir)
	defer cleanupService()

	img1 := image.NewImage(scalar.ToID("1"), "test1.jpg", file1, 100, time.Now(), metadata.NewXMPData(0, "", time.Time{}), 100, 100)
	fakeScanner.Images[filepath.Base(img1.Path())] = img1

	filter := &shared.ImageFilters{Rating: []int{0}}
	sess := NewSession(scalar.ToID("s1"), scalar.ToID("d1"), filter, 10, []*image.Image{img1})

	sess.MarkImage(img1.ID(), shared.ImageActionKeep)

	writeActions := &shared.WriteActions{
		KeepRating:   5,
		ShelveRating: 0,
		RejectRating: -1,
	}

	success, errs := svc.Commit(context.Background(), sess, writeActions)
	require.Empty(t, errs)
	require.Equal(t, 1, success)

	require.Equal(t, 5, fakeMeta.Data[file1].Rating())

	idx := sess.indexByID[img1.ID()]
	inMemImg := sess.images[idx]
	require.Equal(t, 5, inMemImg.Rating(), "In-memory image rating should be updated after commit")
}
