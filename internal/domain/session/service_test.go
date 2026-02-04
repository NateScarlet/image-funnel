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

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Fakes used in TestService_Commit_ShouldOnlyWriteMatchingImages are now in testutil_test.go

// Test Service.Commit
func TestService_Commit_ShouldOnlyWriteMatchingImages(t *testing.T) {
	// Create temp directory and files to satisfy os.Stat
	tempDir, err := os.MkdirTemp("", "session_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create 3 dummy files
	file1 := filepath.Join(tempDir, "test1.jpg")
	file2 := filepath.Join(tempDir, "test2.jpg")
	file3 := filepath.Join(tempDir, "test3.jpg")
	os.WriteFile(file1, []byte("fake"), 0644)
	os.WriteFile(file2, []byte("fake"), 0644)
	os.WriteFile(file3, []byte("fake"), 0644)

	// Fakes
	fakeMeta := NewFakeMetadataRepo()
	fakeSessionRepo := NewFakeSessionRepo()
	fakeEventBus := &FakeEventBus{}

	// Real in-memory topic
	topic, cleanup := pubsub.NewInMemoryTopic[*Session]()
	defer cleanup()

	svc, cleanupService := NewService(fakeSessionRepo, fakeMeta, nil, fakeEventBus, zap.NewNop(), topic, "")
	defer cleanupService()

	// Prepare Session
	filter := &shared.ImageFilters{Rating: []int{0}}

	img1 := image.NewImage(scalar.ToID("1"), "test1.jpg", file1, 100, time.Now(), metadata.NewXMPData(0, "", time.Time{}), 100, 100)
	img2 := image.NewImage(scalar.ToID("2"), "test2.jpg", file2, 100, time.Now(), metadata.NewXMPData(5, "", time.Time{}), 100, 100)
	img3 := image.NewImage(scalar.ToID("3"), "test3.jpg", file3, 100, time.Now(), metadata.NewXMPData(0, "", time.Time{}), 100, 100)

	sess := NewSession(scalar.ToID("s1"), scalar.ToID("d1"), filter, 10, []*image.Image{img1, img3})

	// Inject actions/images
	sess.MarkImage(img1.ID(), shared.ImageActionKeep)
	// img2 is not in session (filtered out by initial creation logic if we strictly followed logic, but here we construct abstractly)
	// But let's manually add it to internal maps to simulate "history" if needed,
	// or just realize that Commit iterates session.Actions().

	// To strictly simulate "previous actions" on images not in current queue, we need access to internals or use methods.
	// UpdateImageByPath might work but it respects matchesFilter.
	// Here we can directly manipulate if we are in the same package.
	// Valid since package session logic allows unit tests access to privates.
	// But since we are in `package session`, we can access `sess.images` etc.
	sess.mu.Lock()
	sess.images[img2.ID()] = img2
	sess.actions[img2.ID()] = shared.ImageActionKeep // Action exists but image doesn't match filter (Rating 5 vs Filter 0)

	sess.images[img3.ID()] = img3
	sess.actions[img3.ID()] = shared.ImageActionReject
	sess.mu.Unlock()

	writeActions := &shared.WriteActions{
		KeepRating:   5,
		ShelveRating: 0,
		RejectRating: -1,
	}

	// Execute Commit
	success, errs := svc.Commit(context.Background(), sess, writeActions)

	require.Empty(t, errs)
	require.Equal(t, 2, success, "Should successfully write 2 images")

	// Validate FakeMeta
	// Image 1: Matches -> Should be written
	require.Contains(t, fakeMeta.Data, file1)
	require.Equal(t, 5, fakeMeta.Data[file1].Rating())
	require.Equal(t, shared.ImageActionKeep.String(), fakeMeta.Data[file1].Action())

	// Image 2: Does not match -> Should NOT be written
	require.NotContains(t, fakeMeta.Data, file2)

	// Image 3: Matches (Rating 0) -> Should be written
	require.Contains(t, fakeMeta.Data, file3)
	require.Equal(t, -1, fakeMeta.Data[file3].Rating())
	require.Equal(t, shared.ImageActionReject.String(), fakeMeta.Data[file3].Action())
}
