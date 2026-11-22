package note

import (
	"context"
	"iter"
	"sync"
	"time"

	domnote "main/internal/domain/note"
	"main/internal/domain/relpath"
	"main/internal/pubsub"

	"go.uber.org/zap"
)

const (
	// finalizeDedupWindow 同一笔记在终结后短时间内到达的重复终结事件的去重窗口。
	// 文件系统监视器的迟到事件在这段窗口内到达时不再发布第二条保存完成事件。
	finalizeDedupWindow = 200 * time.Millisecond

	// DefaultSavedEventTimeout 等待钩子链路终结的兜底超时。
	// 覆盖「文件变更事件上报」的耗时；超过仍未终结说明事件可能丢失，才兜底发布。
	DefaultSavedEventTimeout = 10 * time.Second

	// processingSafetyTimeout 钩子已开始处理时的安全超时。
	// 钩子脚本可能远超 DefaultSavedEventTimeout，期间不得兜底发布中间内容；
	// 仅在钩子异常退出、始终未终结时，才用更长的窗口兜底。
	processingSafetyTimeout = 5 * time.Minute
)

// savedEvents 笔记保存完成事件的发布协调器。
//
// 应用层保存调用 Saved 登记「待终结」，钩子链路开始处理时调用 Processing 暂停兜底计时，
// 全部写入完成后调用 Finalized 终结并发布，保证订阅端只在最终内容落位后才收到事件。
type savedEvents struct {
	pub     pubsub.Topic[*domnote.SavedEvent]
	timeout time.Duration
	logger  *zap.Logger

	mu      sync.Mutex
	pending map[relpath.Path]*pendingSave
	recent  map[relpath.Path]time.Time
}

// pendingSave 一次等待钩子终结的应用层保存
type pendingSave struct {
	timer *time.Timer
}

func NewSavedEvents(pub pubsub.Topic[*domnote.SavedEvent], timeout time.Duration, logger *zap.Logger) *savedEvents {
	return &savedEvents{
		pub:     pub,
		timeout: timeout,
		logger:  logger,
		pending: make(map[relpath.Path]*pendingSave),
		recent:  make(map[relpath.Path]time.Time),
	}
}

var _ domnote.SavedEvents = (*savedEvents)(nil)

func (s *savedEvents) Saved(_ context.Context, relPath relpath.Path) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 文件事件链路可能先于本方法到达终结；此时该次保存已经发布，不再登记
	if last, ok := s.recent[relPath]; ok && time.Since(last) < finalizeDedupWindow {
		return
	}
	delete(s.recent, relPath)

	if existing, ok := s.pending[relPath]; ok {
		existing.timer.Stop()
	}
	pending := &pendingSave{}
	pending.timer = time.AfterFunc(s.timeout, func() {
		s.publishAfterTimeout(relPath, pending)
	})
	s.pending[relPath] = pending
}

// Processing 钩子链路已接手该笔记：暂停短兜底计时，改用更长的安全超时，
// 避免执行较慢的指令脚本期间把中间内容当作最终内容发布。
func (s *savedEvents) Processing(_ context.Context, relPath relpath.Path) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if pending, ok := s.pending[relPath]; ok {
		pending.timer.Stop()
		pending.timer = time.AfterFunc(processingSafetyTimeout, func() {
			s.publishAfterTimeout(relPath, pending)
		})
	}
}

func (s *savedEvents) Finalized(ctx context.Context, relPath relpath.Path) {
	s.mu.Lock()
	if last, ok := s.recent[relPath]; ok && time.Since(last) < finalizeDedupWindow {
		s.mu.Unlock()
		return
	}
	if pending, ok := s.pending[relPath]; ok {
		pending.timer.Stop()
		delete(s.pending, relPath)
	}
	s.recent[relPath] = time.Now()
	s.mu.Unlock()

	s.publish(ctx, relPath)
}

// publishAfterTimeout 兜底：该保存仍未被钩子终结时发布事件
func (s *savedEvents) publishAfterTimeout(relPath relpath.Path, pending *pendingSave) {
	s.mu.Lock()
	current, ok := s.pending[relPath]
	if !ok || current != pending {
		s.mu.Unlock()
		return
	}
	delete(s.pending, relPath)
	s.recent[relPath] = time.Now()
	s.mu.Unlock()

	s.publish(context.Background(), relPath)
}

// publish 以 fire-and-forget 方式发布：笔记写入已经成功，通知失败不回滚保存，
// 调用方也不等待发布结果，因此失败只记录日志而不向调用方返回错误。
func (s *savedEvents) publish(ctx context.Context, relPath relpath.Path) {
	go func() {
		if err := s.pub.Publish(ctx, &domnote.SavedEvent{RelPath: relPath}); err != nil {
			s.logger.Error("failed to publish note saved event",
				zap.String("relPath", relPath.String()),
				zap.Error(err),
			)
		}
	}()
}

func (s *savedEvents) Subscribe(ctx context.Context) iter.Seq2[*domnote.SavedEvent, error] {
	return s.pub.Subscribe(ctx)
}
