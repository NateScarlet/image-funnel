package directory

import (
	"context"
	"main/internal/domain/directory"
	"main/internal/shared"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestDirEntryDeleted_Throttling(t *testing.T) {
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

	handler := NewHandler(logger, nil, dtoFactory, filterBuilder, repo, dirSvc, pub)
	// 将 batchWindow 缩短到 40ms 方便快速测试
	handler.dirEntryDeletedBatchWindow = 40 * time.Millisecond

	dirA_ID := directory.FromRepository("dir_a").ID()

	results := handler.DirEntryDeleted(ctx, &dirA_ID)

	received := make(chan []*shared.DirEntryDeletedDTO, 10)
	go func() {
		for batch, err := range results {
			if err != nil {
				return
			}
			received <- batch
		}
	}()

	// 1. 发送第 1 个删除事件。期望：立即收到，不等待。
	pub.ch <- &shared.FileChangedEvent{
		DirectoryID: dirA_ID,
		RelPath:     "dir_a/file1.png",
		Action:      shared.FileActionRemove,
	}

	select {
	case batch := <-received:
		if len(batch) != 1 || batch[0].RelPath != "dir_a/file1.png" {
			t.Errorf("expected file1.png immediately, got %v", batch)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for the first immediate event")
	}

	// 2. 在 40ms 窗口期内发送第 2 和第 3 个事件。期望：不立即收到（它们在睡眠限流中）。
	pub.ch <- &shared.FileChangedEvent{
		DirectoryID: dirA_ID,
		RelPath:     "dir_a/file2.png",
		Action:      shared.FileActionRemove,
	}
	pub.ch <- &shared.FileChangedEvent{
		DirectoryID: dirA_ID,
		RelPath:     "dir_a/file3.png",
		Action:      shared.FileActionRename,
	}

	select {
	case batch := <-received:
		t.Fatalf("unexpected immediate batch during sleep: %v", batch)
	default:
		// 正常，没有收到，被限流睡眠挂起
	}

	// 3. 等待睡眠结束。40ms 后睡眠结束，新事件从通道读出并排空，应该把 file2 和 file3 收集并发出来。
	time.Sleep(50 * time.Millisecond)

	select {
	case batch := <-received:
		if len(batch) != 2 {
			t.Errorf("expected batch of 2 elements, got %d", len(batch))
		} else {
			if batch[0].RelPath != "dir_a/file2.png" || batch[1].RelPath != "dir_a/file3.png" {
				t.Errorf("unexpected batch items: %v", batch)
			}
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for the second batch after sleep")
	}

	// 4. 再次睡眠 50ms。这应该把之前的睡眠周期结束并恢复到空闲状态。
	time.Sleep(50 * time.Millisecond)

	// 5. 发送事件。因为已经恢复到空闲状态，应该又是立即收到。
	pub.ch <- &shared.FileChangedEvent{
		DirectoryID: dirA_ID,
		RelPath:     "dir_a/file4.png",
		Action:      shared.FileActionRemove,
	}

	select {
	case batch := <-received:
		if len(batch) != 1 || batch[0].RelPath != "dir_a/file4.png" {
			t.Errorf("expected file4.png immediately, got %v", batch)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for the immediate event after reset")
	}
}
