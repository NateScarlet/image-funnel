package session

import (
	"context"
	"iter"
	"main/internal/domain/directory"
	"main/internal/domain/image"
	"main/internal/domain/metadata"
	"main/internal/domain/relpath"
	"main/internal/pubsub"
	"main/internal/scalar"
	"main/internal/shared"
	"main/internal/testutil"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// 变更事件带的是文件路径，而 XMP 是图片的配套文件。图片本体路径必须由「同一处」
// 同时完成判定与裁剪：漏裁会让 Get 落空，图片既不入队更新、也不触发会话变更通知。
func TestService_HandleFileChange_PairsXMPToImage(t *testing.T) {
	for _, xmpSuffix := range []string{".xmp", ".XMP", ".Xmp"} {
		t.Run(xmpSuffix, func(t *testing.T) {
			rootDir := t.TempDir()
			sess, svc, saved := newFileChangeTestService(t, rootDir, "subdir/test.jpg")

			err := svc.handleFileChange(context.Background(), &directory.FileChangedEvent{
				DirectoryID: scalar.ToID("d1"),
				RelPath:     testutil.RelIn(t, rootDir, "subdir/test.jpg"+xmpSuffix),
				Action:      shared.FileActionWrite,
				OccurredAt:  time.Now(),
			})
			require.NoError(t, err)

			// 定位到图片本体 → UpdateImage 判定为变更 → 发布会话变更通知
			assert.Equal(t, sess.ID(), waitForSaved(t, saved), "配套文件的变更应定位到图片本体")
		})
	}
}

// 非配套文件的后缀不能被误裁：被裁成根目录同样会让 Get 落空、定位不到图片
func TestService_HandleFileChange_NonXMPKeepsItsOwnPath(t *testing.T) {
	rootDir := t.TempDir()
	sess, svc, saved := newFileChangeTestService(t, rootDir, "subdir/test.jpg")

	err := svc.handleFileChange(context.Background(), &directory.FileChangedEvent{
		DirectoryID: scalar.ToID("d1"),
		RelPath:     testutil.RelIn(t, rootDir, "subdir/test.jpg"),
		Action:      shared.FileActionWrite,
		OccurredAt:  time.Now(),
	})
	require.NoError(t, err)

	assert.Equal(t, sess.ID(), waitForSaved(t, saved))
}

func waitForSaved(t *testing.T, saved <-chan scalar.ID) scalar.ID {
	t.Helper()
	select {
	case id := <-saved:
		return id
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for session saved notification")
		return scalar.ID{}
	}
}

// mockSessionSaved 直接捕获发布的会话 ID。
// 真实 InMemoryTopic 的订阅在迭代开始后才注册，用它会引入「注册 vs 发布」的竞态。
type mockSessionSaved struct {
	ids chan scalar.ID
}

func (m *mockSessionSaved) Publish(ctx context.Context, id scalar.ID, opts ...pubsub.PublishOption) error {
	m.ids <- id
	return nil
}

func (m *mockSessionSaved) Subscribe(ctx context.Context) iter.Seq2[scalar.ID, error] {
	return func(yield func(scalar.ID, error) bool) {}
}

// newFileChangeTestService 装配 handleFileChange 所需的全部依赖，
// 并在会话内预置一张位于 relPath 的图片
func newFileChangeTestService(t *testing.T, rootDir, relPath string) (*Session, *Service, <-chan scalar.ID) {
	t.Helper()

	rel := testutil.RelIn(t, rootDir, relPath)
	img := image.New(scalar.ToID("img:1"), "test.jpg", rel, scalar.ToID("d1"), 100, time.Now(),
		metadata.NewXMPData(0, "", time.Time{}, ""), 10, 10)

	sess := New(scalar.ToID("s1"), scalar.ToID("d1"), &shared.ImageFilters{}, 10,
		[]*image.Image{img}, image.NewFilterBuilder())

	metaRepo := NewFakeMetadataRepo()
	imgRepo := &FakeImageRepo{
		MetaRepo: metaRepo,
		BaseDir:  rootDir,
		Images:   map[relpath.Path]*image.Image{rel: img},
	}
	sessionRepo := &FakeSessionRepo{Sessions: map[scalar.ID]*Session{sess.ID(): sess}}
	saved := &mockSessionSaved{ids: make(chan scalar.ID, 4)}

	svc, cleanup := NewService(
		sessionRepo,
		metaRepo,
		imgRepo,
		&mockFileChangedSub{events: make(chan *directory.FileChangedEvent, 1)},
		&mockMetadataUpdatedPub{events: make(chan *shared.MetadataUpdatedEvent, 1)},
		&FakeDirectoryResolver{},
		zap.NewNop(),
		saved,
		rootDir,
		image.NewFilterBuilder(),
		&FakeHookRunner{},
	)
	t.Cleanup(cleanup)
	return sess, svc, saved.ids
}
