package note

import (
	"context"
	"iter"
	"os"
	"path/filepath"
	"testing"
	"time"

	domnote "main/internal/domain/note"
	"main/internal/domain/relpath"
	"main/internal/infrastructure/localfs"
	"main/internal/pubsub"
	"main/internal/scalar"
	"main/internal/shared"
	"main/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// manualSavedEvents 仅在测试需要时投递完成事件，模拟「写入完成后才发布」
type manualSavedEvents struct {
	ch chan *domnote.SavedEvent
}

func (m *manualSavedEvents) Saved(_ context.Context, _ relpath.Path)      {}
func (m *manualSavedEvents) Processing(_ context.Context, _ relpath.Path) {}
func (m *manualSavedEvents) Finalized(_ context.Context, _ relpath.Path)  {}
func (m *manualSavedEvents) Subscribe(ctx context.Context) iter.Seq2[*domnote.SavedEvent, error] {
	return func(yield func(*domnote.SavedEvent, error) bool) {
		for {
			select {
			case ev := <-m.ch:
				if !yield(ev, nil) {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}
}

func (m *manualSavedEvents) emit(rel relpath.Path) {
	m.ch <- &domnote.SavedEvent{RelPath: rel}
}

// TestNoteSaved_ReadsFinalContentOnCompletionEvent 完成事件到达时读取的是磁盘上的最终内容
func TestNoteSaved_ReadsFinalContentOnCompletionEvent(t *testing.T) {
	root := t.TempDir()
	repo := localfs.NewNoteRepository(root)
	rel := testutil.RelIn(t, root, "topic/final.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(rel.Abs(root)), 0755))

	// 先写中间内容，再写最终内容；只有最终内容落位后才发布完成事件
	require.NoError(t, os.WriteFile(rel.Abs(root), []byte("intermediate content"), 0644))
	require.NoError(t, os.WriteFile(rel.Abs(root), []byte("---\nhidden: false\n---\nfinal content"), 0644))

	saved := &manualSavedEvents{ch: make(chan *domnote.SavedEvent, 4)}
	h := NewHandler(
		zap.NewNop(),
		repo,
		domnote.NewService(repo, domnote.NewFactory(root), root),
		nil,
		nil,
		NewDTOFactory(root),
		domnote.NewFilterBuilder(),
		saved,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results := make(chan *shared.NoteDTO, 4)
	go func() {
		for dto, err := range h.NoteSaved(ctx, nil) {
			if err != nil || dto == nil {
				continue
			}
			results <- dto
		}
	}()

	saved.emit(rel)

	select {
	case dto := <-results:
		assert.Equal(t, "final content", dto.Content)
		assert.Equal(t, "---\nhidden: false\n---\nfinal content", dto.RawContent)
	case <-time.After(2 * time.Second):
		t.Fatal("等待 NoteSaved 事件超时")
	}
}

// TestNoteSaved_FiltersByNoteID 按笔记 ID 过滤，非目标笔记的完成事件被忽略
func TestNoteSaved_FiltersByNoteID(t *testing.T) {
	root := t.TempDir()
	repo := localfs.NewNoteRepository(root)
	target := testutil.RelIn(t, root, "topic/target.md")
	other := testutil.RelIn(t, root, "topic/other.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(target.Abs(root)), 0755))
	require.NoError(t, os.WriteFile(target.Abs(root), []byte("target content"), 0644))
	require.NoError(t, os.WriteFile(other.Abs(root), []byte("other content"), 0644))

	saved := &manualSavedEvents{ch: make(chan *domnote.SavedEvent, 4)}
	h := NewHandler(
		zap.NewNop(),
		repo,
		domnote.NewService(repo, domnote.NewFactory(root), root),
		nil,
		nil,
		NewDTOFactory(root),
		domnote.NewFilterBuilder(),
		saved,
	)

	targetID := domnote.FromRepository(target, target.Abs(root), "target content", time.Now()).ID()
	filter := &shared.NoteFilters{ID: []scalar.ID{targetID}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results := make(chan *shared.NoteDTO, 4)
	go func() {
		for dto, err := range h.NoteSaved(ctx, filter) {
			if err != nil || dto == nil {
				continue
			}
			results <- dto
		}
	}()

	saved.emit(other)
	saved.emit(target)

	select {
	case dto := <-results:
		assert.Equal(t, targetID, dto.ID)
	case <-time.After(2 * time.Second):
		t.Fatal("等待目标笔记的 NoteSaved 事件超时")
	}
	assert.Empty(t, results, "非目标笔记的完成事件不应产生通知")
}

// TestNoteSaved_ChainPublishesFinalContentOnce 串联真实协调器、主题与订阅：
// 写入完成后登记，钩子处理并终结，迟到的终结事件不得造成第二次通知
func TestNoteSaved_ChainPublishesFinalContentOnce(t *testing.T) {
	root := t.TempDir()
	repo := localfs.NewNoteRepository(root)
	rel := testutil.RelIn(t, root, "chain/note.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(rel.Abs(root)), 0755))
	require.NoError(t, os.WriteFile(rel.Abs(root), []byte("draft content"), 0644))

	topic, closeTopic := pubsub.NewInMemoryTopic[*domnote.SavedEvent]()
	defer closeTopic()
	coordinator := NewSavedEvents(topic, time.Minute, zap.NewNop())
	h := NewHandler(
		zap.NewNop(),
		repo,
		domnote.NewService(repo, domnote.NewFactory(root), root),
		nil,
		nil,
		NewDTOFactory(root),
		domnote.NewFilterBuilder(),
		coordinator,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results := make(chan *shared.NoteDTO, 4)
	go func() {
		for dto, err := range h.NoteSaved(ctx, nil) {
			if err != nil || dto == nil {
				continue
			}
			results <- dto
		}
	}()
	// 等待订阅在事件总线上注册完成（InMemoryTopic 不重放订阅前的事件）
	time.Sleep(100 * time.Millisecond)

	// 模拟应用层写入完成 → 钩子接手 → 全部写入完成 → 迟到事件
	require.NoError(t, os.WriteFile(rel.Abs(root), []byte("final content"), 0644))
	coordinator.Saved(ctx, rel)
	coordinator.Processing(ctx, rel)
	coordinator.Finalized(ctx, rel)
	coordinator.Finalized(ctx, rel)

	select {
	case dto := <-results:
		assert.Equal(t, "final content", dto.Content)
	case <-time.After(2 * time.Second):
		t.Fatal("等待完整链路的 NoteSaved 事件超时")
	}
	select {
	case dto := <-results:
		t.Fatalf("一次保存不应产生第二次通知，收到: %+v", dto)
	case <-time.After(300 * time.Millisecond):
	}
}
