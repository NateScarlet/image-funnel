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
func TestService_Commit_ShouldWriteAllActions(t *testing.T) {
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
	// img2 has Rating 3, so it does NOT match the filter (Rating 0)
	img2 := image.NewImage(scalar.ToID("2"), "test2.jpg", file2, 100, time.Now(), metadata.NewXMPData(3, "", time.Time{}), 100, 100)
	img3 := image.NewImage(scalar.ToID("3"), "test3.jpg", file3, 100, time.Now(), metadata.NewXMPData(0, "", time.Time{}), 100, 100)

	sess := NewSession(scalar.ToID("s1"), scalar.ToID("d1"), filter, 10, []*image.Image{img1, img3})

	// Inject actions/images
	sess.MarkImage(img1.ID(), shared.ImageActionKeep)

	// 手动注入 img2（因为 img2 不符合初始过滤器的 Rating 0 条件，所以初始化时会被过滤，
	// 但我们需要模拟一种场景：img2 在之前的轮次或过滤器下被标记了 KEEP，
	// 之后过滤器变更为只显示 rating=0 的图片，导致 img2 变得不可见。
	// 此时提交应仍然包含 img2 的修改。）
	sess.mu.Lock()
	sess.images[img2.ID()] = img2
	sess.actions[img2.ID()] = shared.ImageActionKeep
	sess.mu.Unlock()

	// 标记 img3 为 REJECT
	// 注意：虽然使用 sess.MarkImage 更符合业务流程，但测试重点在于 Commit
	// 对已有操作的处理，所以这里也采用直接修改内部状态的方式来确保测试条件精确。
	sess.mu.Lock()
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
	// Should match 3: img1 (Keep->5), img2 (Keep->5, even if hidden), img3 (Reject->-1)
	require.Equal(t, 3, success, "Should successfully write all 3 images")

	// Validate FakeMeta
	// Image 1: Matches -> Should be written
	require.Contains(t, fakeMeta.Data, file1)
	require.Equal(t, 5, fakeMeta.Data[file1].Rating())
	require.Equal(t, shared.ImageActionKeep.String(), fakeMeta.Data[file1].Action())

	// Image 2: Does not match filter, BUT matches action -> Should be written now
	require.Contains(t, fakeMeta.Data, file2)
	require.Equal(t, 5, fakeMeta.Data[file2].Rating())
	require.Equal(t, shared.ImageActionKeep.String(), fakeMeta.Data[file2].Action())

	// Image 3: Matches (Rating 0) -> Should be written
	require.Contains(t, fakeMeta.Data, file3)
	require.Equal(t, -1, fakeMeta.Data[file3].Rating())
	require.Equal(t, shared.ImageActionReject.String(), fakeMeta.Data[file3].Action())
}
