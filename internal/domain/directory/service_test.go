package directory

import (
	"context"
	"iter"
	"main/internal/domain/relpath"
	"main/internal/pubsub"
	"main/internal/shared"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

// testRootDir 测试用的虚拟根目录，仅用于根相对路径的围栏，不实际访问磁盘
const testRootDir = "C:/mock_root"

type mockWatcher struct {
	changes chan *FileChange
}

func (w *mockWatcher) Watch(ctx context.Context, dir string) iter.Seq2[*FileChange, error] {
	return func(yield func(*FileChange, error) bool) {
		for {
			select {
			case <-ctx.Done():
				return
			case change := <-w.changes:
				if !yield(change, nil) {
					return
				}
			}
		}
	}
}

type mockFileChangedPub struct {
	events chan *FileChangedEvent
}

func (e *mockFileChangedPub) Publish(ctx context.Context, event *FileChangedEvent, opts ...pubsub.PublishOption) error {
	e.events <- event
	return nil
}

func (e *mockFileChangedPub) Subscribe(ctx context.Context) iter.Seq2[*FileChangedEvent, error] {
	return func(yield func(*FileChangedEvent, error) bool) {}
}

type mockRepository struct {
	findErr error
	findRes []*Directory
}

func (m *mockRepository) Get(ctx context.Context, relPath relpath.Path) (*Directory, error) {
	return FromRepository(relPath), nil
}

func (m *mockRepository) Find(ctx context.Context, relPath relpath.Path) iter.Seq2[*Directory, error] {
	return func(yield func(*Directory, error) bool) {
		if m.findErr != nil {
			yield(nil, m.findErr)
			return
		}
		for _, dir := range m.findRes {
			if !yield(dir, nil) {
				return
			}
		}
	}
}

func (m *mockRepository) ReadState(ctx context.Context, relPath relpath.Path) (*shared.DirectoryStateDTO, error) {
	return nil, nil
}

func (m *mockRepository) WriteState(ctx context.Context, relPath relpath.Path, state *shared.DirectoryStateDTO) error {
	return nil
}

func TestService_SuggestDirectories_IgnoreNotExist(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	repo := &mockRepository{
		findErr: os.ErrNotExist,
	}
	watcher := &mockWatcher{changes: make(chan *FileChange)}
	ebus := &mockFileChangedPub{events: make(chan *FileChangedEvent, 1)}
	s, cleanup := NewService(watcher, ebus, testRootDir, repo, &mockRenamer{}, logger)
	defer cleanup()

	// 尽管 Find 报错 fs.ErrNotExist，SuggestDirectories 应当吞掉此错误并返回空列表
	input := shared.PathInput{
		RelativeToRoot: "non_existent_path/sub",
	}

	count := 0
	for _, err := range s.SuggestDirectories(ctx, relpath.Path{}, input) {
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		count++
	}

	if count != 0 {
		t.Errorf("expected 0 directories, got %d", count)
	}
}

// 监听器产出的是绝对路径，必须先转为根相对路径再发布；
// 这条链路一旦断了，文件变更就完全到不了会话与钩子，但此前没有任何测试覆盖它
func TestService_WatchAndTransform(t *testing.T) {
	watcher := &mockWatcher{changes: make(chan *FileChange)}
	ebus := &mockFileChangedPub{events: make(chan *FileChangedEvent, 1)}
	_, cleanup := NewService(watcher, ebus, testRootDir, &mockRepository{}, &mockRenamer{}, zap.NewNop())
	defer cleanup()

	watcher.changes <- NewFileChange(filepath.Join(testRootDir, "subdir", "test.jpg"), shared.FileActionWrite, time.Now())

	select {
	case event := <-ebus.events:
		assert.Equal(t, "subdir/test.jpg", event.RelPath.String())
		assert.Equal(t, dirAt(t, "subdir").ID(), event.DirectoryID)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for the transformed file changed event")
	}
}

// 根目录之外的上报路径无法被转成根相对路径，只记录日志、不发布事件
func TestService_WatchAndTransform_OutOfRootIsNotPublished(t *testing.T) {
	watcher := &mockWatcher{changes: make(chan *FileChange)}
	ebus := &mockFileChangedPub{events: make(chan *FileChangedEvent, 1)}
	_, cleanup := NewService(watcher, ebus, testRootDir, &mockRepository{}, &mockRenamer{}, zap.NewNop())
	defer cleanup()

	watcher.changes <- NewFileChange(filepath.Join(testRootDir, "..", "outside.jpg"), shared.FileActionWrite, time.Now())

	select {
	case event := <-ebus.events:
		t.Fatalf("越界路径不应被发布为事件: %q", event.RelPath)
	case <-time.After(200 * time.Millisecond):
	}
}

// 路径输入由用户提供，越界的输入无法被构造出来：
// 「移动到根目录外」的防线由 ResolvePathInput 承担，下游移动方无需重复校验
func TestService_ResolvePathInput(t *testing.T) {
	ctx := context.Background()
	s, cleanup := NewService(&mockWatcher{}, &mockFileChangedPub{}, testRootDir, &mockRepository{}, &mockRenamer{}, zap.NewNop())
	defer cleanup()

	current := dirAt(t, "current").RelPath()

	accepted := map[string]struct {
		input shared.PathInput
		want  string
	}{
		"relative to root": {
			input: shared.PathInput{RelativeToRoot: "target"},
			want:  "target",
		},
		"relative to current": {
			input: shared.PathInput{RelativeToCurrent: "nested"},
			want:  "current/nested",
		},
		"absolute inside root": {
			input: shared.PathInput{Absolute: filepath.Join(testRootDir, "target")},
			want:  "target",
		},
	}
	for name, tt := range accepted {
		t.Run(name, func(t *testing.T) {
			got, err := s.ResolvePathInput(ctx, current, tt.input)
			if err != nil {
				t.Fatalf("ResolvePathInput() unexpected error: %v", err)
			}
			if got.String() != tt.want {
				t.Errorf("ResolvePathInput() = %q, want %q", got, tt.want)
			}
		})
	}

	rejected := map[string]shared.PathInput{
		"escaping relative to root":    {RelativeToRoot: "../outside"},
		"escaping relative to current": {RelativeToCurrent: "../../outside"},
		"absolute outside root":        {Absolute: filepath.Join(testRootDir, "..", "outside")},
		"empty input":                  {},
		"multiple modes":               {RelativeToRoot: "a", RelativeToCurrent: "b"},
	}
	for name, input := range rejected {
		t.Run(name, func(t *testing.T) {
			if _, err := s.ResolvePathInput(ctx, current, input); err == nil {
				t.Fatalf("期望 %+v 被拒绝，但解析成功", input)
			}
		})
	}
}
