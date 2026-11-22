package note

import (
	"context"
	"iter"
	"sync"
	"testing"
	"time"

	domnote "main/internal/domain/note"
	"main/internal/pubsub"
	"main/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// recordingSavedTopic 记录发布次数的事件总线替身
type recordingSavedTopic struct {
	mu     sync.Mutex
	events []*domnote.SavedEvent
}

func (r *recordingSavedTopic) Publish(_ context.Context, v *domnote.SavedEvent, _ ...pubsub.PublishOption) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, v)
	return nil
}

func (r *recordingSavedTopic) Subscribe(_ context.Context) iter.Seq2[*domnote.SavedEvent, error] {
	return func(func(*domnote.SavedEvent, error) bool) {}
}

func (r *recordingSavedTopic) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events)
}

func (r *recordingSavedTopic) first() *domnote.SavedEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.events) == 0 {
		return nil
	}
	return r.events[0]
}

func newSavedEventsForTest(topic *recordingSavedTopic, timeout time.Duration) *savedEvents {
	return NewSavedEvents(topic, timeout, zap.NewNop())
}

func waitForCount(t *testing.T, topic *recordingSavedTopic, want int) {
	t.Helper()
	require.Eventually(t, func() bool { return topic.count() == want }, time.Second, 5*time.Millisecond)
}

// TestSavedEvents_FinalizeAfterSavedPublishesOnce 应用保存登记后由钩子终结，只发布一次
func TestSavedEvents_FinalizeAfterSavedPublishesOnce(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	rel := testutil.RelIn(t, root, "dir/note.md")
	topic := &recordingSavedTopic{}
	events := newSavedEventsForTest(topic, time.Minute)

	events.Saved(ctx, rel)
	assert.Zero(t, topic.count(), "终结点到达前不应发布事件")

	events.Finalized(ctx, rel)
	waitForCount(t, topic, 1)
	assert.Equal(t, rel, topic.first().RelPath)
}

// TestSavedEvents_FinalizeDeduplicatesLateEvents 迟到的重复终结不产生重复通知
func TestSavedEvents_FinalizeDeduplicatesLateEvents(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	rel := testutil.RelIn(t, root, "note.md")
	topic := &recordingSavedTopic{}
	events := newSavedEventsForTest(topic, time.Minute)

	events.Saved(ctx, rel)
	events.Finalized(ctx, rel)
	events.Finalized(ctx, rel)
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 1, topic.count(), "同一次保存的迟到终结事件必须被抑制")
}

// TestSavedEvents_ExternalFinalizePublishes 外部编辑器修改（未经 Saved 登记）仍发布事件
func TestSavedEvents_ExternalFinalizePublishes(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	rel := testutil.RelIn(t, root, "external.md")
	topic := &recordingSavedTopic{}
	events := newSavedEventsForTest(topic, time.Minute)

	events.Finalized(ctx, rel)
	waitForCount(t, topic, 1)
	assert.Equal(t, rel, topic.first().RelPath)
}

// TestSavedEvents_TimeoutFallbackPublishes 钩子链路未接手时超时兜底发布
func TestSavedEvents_TimeoutFallbackPublishes(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	rel := testutil.RelIn(t, root, "fallback.md")
	topic := &recordingSavedTopic{}
	events := newSavedEventsForTest(topic, 30*time.Millisecond)

	events.Saved(ctx, rel)
	waitForCount(t, topic, 1)
}

// TestSavedEvents_ProcessingPausesTimeoutFallback 钩子开始处理后不因慢脚本而提前发布中间内容
func TestSavedEvents_ProcessingPausesTimeoutFallback(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	rel := testutil.RelIn(t, root, "slow-hook.md")
	topic := &recordingSavedTopic{}
	events := newSavedEventsForTest(topic, 30*time.Millisecond)

	events.Saved(ctx, rel)
	events.Processing(ctx, rel)
	time.Sleep(100 * time.Millisecond)
	assert.Zero(t, topic.count(), "钩子处理期间不得因兜底超时发布中间内容")

	events.Finalized(ctx, rel)
	waitForCount(t, topic, 1)
}

// TestSavedEvents_FinalizedBeforeSavedDoesNotDuplicate 文件链路先终结、后登记时不会重复发布
func TestSavedEvents_FinalizedBeforeSavedDoesNotDuplicate(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	rel := testutil.RelIn(t, root, "race.md")
	topic := &recordingSavedTopic{}
	events := newSavedEventsForTest(topic, 30*time.Millisecond)

	events.Finalized(ctx, rel)
	waitForCount(t, topic, 1)

	// UpdateNote 中 Save 之后的登记晚于文件事件链路的终结
	events.Saved(ctx, rel)
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, 1, topic.count(), "迟到的 Saved 登记不应造成第二次通知")
}

// TestSavedEvents_TimeoutFinalizedByHookDoesNotDuplicate 已被钩子终结的保存不再触发兜底发布
func TestSavedEvents_TimeoutFinalizedByHookDoesNotDuplicate(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	rel := testutil.RelIn(t, root, "timely.md")
	topic := &recordingSavedTopic{}
	events := newSavedEventsForTest(topic, 200*time.Millisecond)

	events.Saved(ctx, rel)
	events.Finalized(ctx, rel)
	time.Sleep(300 * time.Millisecond)
	assert.Equal(t, 1, topic.count())
}

// TestSavedEvents_DifferentPathsAreIndependent 不同笔记的保存互不干扰
func TestSavedEvents_DifferentPathsAreIndependent(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	first := testutil.RelIn(t, root, "first.md")
	second := testutil.RelIn(t, root, "second.md")
	topic := &recordingSavedTopic{}
	events := newSavedEventsForTest(topic, time.Minute)

	events.Saved(ctx, first)
	events.Saved(ctx, second)
	events.Finalized(ctx, first)
	events.Finalized(ctx, second)

	waitForCount(t, topic, 2)
}
