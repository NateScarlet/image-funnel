package directory

import (
	"context"
	"iter"
	"main/internal/domain/directory"
	"main/internal/pubsub"
	"main/internal/shared"
	"testing"
	"time"

	"go.uber.org/zap"
)

type mockWatcher struct{}

func (w *mockWatcher) Watch(ctx context.Context, dir string) iter.Seq2[*directory.FileChange, error] {
	return func(yield func(*directory.FileChange, error) bool) {}
}

type mockFileChangedPub struct {
	ch chan *shared.FileChangedEvent
}

func (m *mockFileChangedPub) Publish(ctx context.Context, event *shared.FileChangedEvent, opts ...pubsub.PublishOption) error {
	select {
	case m.ch <- event:
	default:
	}
	return nil
}

func (m *mockFileChangedPub) Subscribe(ctx context.Context) iter.Seq2[*shared.FileChangedEvent, error] {
	return func(yield func(*shared.FileChangedEvent, error) bool) {
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-m.ch:
				if !ok {
					return
				}
				if !yield(ev, nil) {
					return
				}
			}
		}
	}
}

type mockRepository struct {
	dirs map[string]*directory.Directory
}

func (m *mockRepository) Get(ctx context.Context, relPath string) (*directory.Directory, error) {
	dir, ok := m.dirs[relPath]
	if !ok {
		dir = directory.FromRepository(relPath)
		m.dirs[relPath] = dir
	}
	return dir, nil
}

func (m *mockRepository) Find(ctx context.Context, relPath string) iter.Seq2[*directory.Directory, error] {
	return func(yield func(*directory.Directory, error) bool) {}
}

func (m *mockRepository) ReadState(ctx context.Context, relPath string) (*shared.DirectoryStateDTO, error) {
	return nil, nil
}

func (m *mockRepository) WriteState(ctx context.Context, relPath string, state *shared.DirectoryStateDTO) error {
	return nil
}

func TestDirectoryChanged_NoThrottle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := zap.NewNop()
	repo := &mockRepository{dirs: make(map[string]*directory.Directory)}
	watcher := &mockWatcher{}
	pub := &mockFileChangedPub{ch: make(chan *shared.FileChangedEvent, 100)}

	dirSvc, cleanup := directory.NewService(watcher, pub, "C:/mock_root", repo, logger)
	defer cleanup()

	dtoFactory := NewDTOFactory(nil)
	filterBuilder := directory.NewFilterBuilder()

	handler := NewHandler(nil, dtoFactory, filterBuilder, repo, dirSvc, pub)

	results := handler.DirectoryChanged(ctx, shared.DirectoryFilters{}, DirectoryChangedWithThrottle(0))

	received := make(chan *shared.DirectoryDTO, 10)
	go func() {
		for dto, err := range results {
			if err != nil {
				return
			}
			received <- dto
		}
	}()

	pub.ch <- &shared.FileChangedEvent{
		DirectoryID: directory.FromRepository("test_dir").ID(),
		RelPath:     "test_dir/file1.png",
	}

	select {
	case dto := <-received:
		if dto.RelPath != "test_dir" {
			t.Errorf("expected test_dir, got %s", dto.RelPath)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for event in unthrottled mode")
	}
}

func TestDirectoryChanged_WithThrottle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := zap.NewNop()
	repo := &mockRepository{dirs: make(map[string]*directory.Directory)}
	watcher := &mockWatcher{}
	pub := &mockFileChangedPub{ch: make(chan *shared.FileChangedEvent, 100)}

	dirSvc, cleanup := directory.NewService(watcher, pub, "C:/mock_root", repo, logger)
	defer cleanup()

	dtoFactory := NewDTOFactory(nil)
	filterBuilder := directory.NewFilterBuilder()

	handler := NewHandler(nil, dtoFactory, filterBuilder, repo, dirSvc, pub)

	throttleTime := 40 * time.Millisecond
	results := handler.DirectoryChanged(ctx, shared.DirectoryFilters{}, DirectoryChangedWithThrottle(throttleTime))

	received := make(chan *shared.DirectoryDTO, 10)
	go func() {
		for dto, err := range results {
			if err != nil {
				return
			}
			received <- dto
		}
	}()

	dirA_ID := directory.FromRepository("dir_a").ID()
	dirB_ID := directory.FromRepository("dir_b").ID()

	// T0: 发送目录 A. 期望：立即收到 A (Leading)
	pub.ch <- &shared.FileChangedEvent{
		DirectoryID: dirA_ID,
		RelPath:     "dir_a/file1.png",
	}

	select {
	case dto := <-received:
		if dto.RelPath != "dir_a" {
			t.Errorf("T0: expected dir_a, got %s", dto.RelPath)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("T0: timeout waiting for leading edge of dir_a")
	}

	// T0 + 10ms: 在冷却期内发送目录 A。期望：被缓存，不立即触发
	time.Sleep(10 * time.Millisecond)
	pub.ch <- &shared.FileChangedEvent{
		DirectoryID: dirA_ID,
		RelPath:     "dir_a/file2.png",
	}
	select {
	case dto := <-received:
		t.Fatalf("T0+10ms: unexpected event received in cooling time: %s", dto.RelPath)
	default:
		// 正常：没有收到任何事件
	}

	// T0 + 15ms: 在冷却期内发送不同的目录 B。期望：立即收到 B (Leading，因为不同目录独立节流)
	time.Sleep(5 * time.Millisecond)
	pub.ch <- &shared.FileChangedEvent{
		DirectoryID: dirB_ID,
		RelPath:     "dir_b/file1.png",
	}
	select {
	case dto := <-received:
		if dto.RelPath != "dir_b" {
			t.Errorf("T0+15ms: expected dir_b, got %s", dto.RelPath)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("T0+15ms: timeout waiting for leading edge of dir_b")
	}

	// T0 + 20ms: 再次在 A 的冷却期内发送目录 A（更新挂起值）
	time.Sleep(5 * time.Millisecond)
	pub.ch <- &shared.FileChangedEvent{
		DirectoryID: dirA_ID,
		RelPath:     "dir_a/file3.png",
	}
	select {
	case dto := <-received:
		t.Fatalf("T0+20ms: unexpected event received: %s", dto.RelPath)
	default:
		// 正常
	}

	// 此时对于目录 A，我们在冷却期内发送了 file2 (10ms) 和 file3 (20ms)。
	// 在 T0 + 40ms（大约）时，A 的冷却定时器应该到期。
	// 期望：收到 A 的 Trailing 触发。
	// 为了确保定时器被处理完，我们 sleep 到 55ms。
	time.Sleep(35 * time.Millisecond)
	select {
	case dto := <-received:
		if dto.RelPath != "dir_a" {
			t.Errorf("T0+55ms: expected dir_a (trailing), got %s", dto.RelPath)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("T0+55ms: timeout waiting for trailing edge of dir_a")
	}

	// 再次确认之后，此时通道应该是空的。
	select {
	case dto := <-received:
		t.Fatalf("unexpected trailing event: %s", dto.RelPath)
	default:
		// 正常
	}
}
