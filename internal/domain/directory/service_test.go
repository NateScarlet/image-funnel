package directory

import (
	"context"
	"iter"
	"main/internal/shared"
	"os"
	"testing"

	"go.uber.org/zap"
)

type mockWatcher struct{}

func (w *mockWatcher) Watch(ctx context.Context, dir string) iter.Seq2[*FileChange, error] {
	return func(yield func(*FileChange, error) bool) {}
}

type mockEventBus struct{}

func (e *mockEventBus) PublishFileChanged(ctx context.Context, event *shared.FileChangedEvent) {}

type mockRepository struct {
	findErr error
	findRes []*Directory
}

func (m *mockRepository) Get(ctx context.Context, relPath string) (*Directory, error) {
	return FromRepository(relPath), nil
}

func (m *mockRepository) Find(ctx context.Context, relPath string) iter.Seq2[*Directory, error] {
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

func (m *mockRepository) ReadState(ctx context.Context, relPath string) (*shared.DirectoryStateDTO, error) {
	return nil, nil
}

func (m *mockRepository) WriteState(ctx context.Context, relPath string, state *shared.DirectoryStateDTO) error {
	return nil
}

func TestService_SuggestDirectories_IgnoreNotExist(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	repo := &mockRepository{
		findErr: os.ErrNotExist,
	}
	watcher := &mockWatcher{}
	ebus := &mockEventBus{}

	s, cleanup := NewService(watcher, ebus, "C:/mock_root", repo, logger)
	defer cleanup()

	// 尽管 Find 报错 fs.ErrNotExist，SuggestDirectories 应当吞掉此错误并返回空列表
	input := shared.PathInput{
		RelativeToRoot: "non_existent_path/sub",
	}

	count := 0
	for _, err := range s.SuggestDirectories(ctx, "", input) {
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		count++
	}

	if count != 0 {
		t.Errorf("expected 0 directories, got %d", count)
	}
}
