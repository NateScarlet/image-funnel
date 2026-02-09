package inmem

import (
	"context"
	"iter"
	"sort"
	"sync"
	"time"

	"main/internal/apperror"
	"main/internal/domain/session"
	"main/internal/scalar"
)

const (
	// minRetainedSessions 保留最近的会话数量，无论是否过期
	minRetainedSessions = 10
	// maxSessionIdleTime 会话最大空闲时间，超过此时间且不在保留列表中的会话将被清理
	maxSessionIdleTime = 24 * time.Hour
)

// sessionOwnership 表示一个 Session 的所有权控制
type sessionOwnership struct {
	session *session.Session
	// token 信号通道，缓冲为1
	// 谁拿到信号谁就拥有所有权，用完后放回信号
	token chan struct{}
}

type SessionRepository struct {
	sessions        map[scalar.ID]*sessionOwnership
	dirIndex        map[scalar.ID][]scalar.ID
	mu              sync.RWMutex
	nextCleanupTime time.Time
}

func NewSessionRepository() *SessionRepository {
	return &SessionRepository{
		sessions: make(map[scalar.ID]*sessionOwnership),
		dirIndex: make(map[scalar.ID][]scalar.ID),
	}
}

// Acquire 获取 Session 的独占访问权
// 阻塞直到拿到信号token
func (r *SessionRepository) Acquire(ctx context.Context, id scalar.ID) (*session.Session, func(), error) {
	r.mu.RLock()
	ownership, exists := r.sessions[id]
	r.mu.RUnlock()

	if !exists {
		return nil, nil, apperror.NewErrDocumentNotFound(id)
	}

	// 等待获取 token
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case <-ownership.token:
		// 拿到 token，获得所有权
	}

	// 创建释放函数，放回 token
	var once sync.Once
	release := func() {
		once.Do(func() {
			ownership.token <- struct{}{}
		})
	}

	return ownership.session, release, nil
}

// Create 创建新 Session 并返回释放函数
// 调用者在创建后可能需要继续操作，因此持有锁直到调用 release
func (r *SessionRepository) Create(sess *session.Session) (func(), error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := sess.ID()
	if _, exists := r.sessions[id]; exists {
		return nil, apperror.New("SESSION_ALREADY_EXISTS", "session already exists: "+id.String(), "会话已存在")
	}

	// 创建新的 ownership，初始化 token（但不放入信号，因为创建者持有）
	token := make(chan struct{}, 1)

	r.sessions[id] = &sessionOwnership{
		session: sess,
		token:   token,
	}

	// 更新目录索引
	dirID := sess.DirectoryID()
	r.dirIndex[dirID] = append(r.dirIndex[dirID], id)

	// 触发清理机制
	r.cleanup()

	// 返回释放函数
	var once sync.Once
	release := func() {
		once.Do(func() {
			token <- struct{}{}
		})
	}

	return release, nil
}

func (r *SessionRepository) FindByDirectory(directoryID scalar.ID) iter.Seq2[scalar.ID, error] {
	return func(yield func(scalar.ID, error) bool) {
		r.mu.RLock()
		ids := make([]scalar.ID, len(r.dirIndex[directoryID]))
		copy(ids, r.dirIndex[directoryID])
		r.mu.RUnlock()

		for _, id := range ids {
			if !yield(id, nil) {
				return
			}
		}
	}
}

// cleanup 清理长时间未更新的会话
// 注意：此方法必须在持有写锁的情况下调用
func (r *SessionRepository) cleanup() {
	now := time.Now()
	// 优化：如果还没到下一次清理时间，直接返回
	if now.Before(r.nextCleanupTime) {
		return
	}

	total := len(r.sessions)
	if total <= minRetainedSessions {
		// 如果未达到最小保留数，无需清理
		return
	}

	threshold := now.Add(-maxSessionIdleTime)

	var candidates []*sessionOwnership
	var oldestActiveTime time.Time

	for _, ownership := range r.sessions {
		select {
		case <-ownership.token:
			// 成功获取 token，说明当前没有人在使用，可以安全读取 session 状态
			sess := ownership.session
			updatedAt := sess.UpdatedAt()
			if !updatedAt.After(threshold) {
				// 已过期，加入候选列表（注意：此时我们仍持有其 token）
				candidates = append(candidates, ownership)
			} else {
				// 未过期，放回 token
				ownership.token <- struct{}{}
				// 记录活跃会话中最早的更新时间，用于计算下次清理时间
				if oldestActiveTime.IsZero() || updatedAt.Before(oldestActiveTime) {
					oldestActiveTime = updatedAt
				}
			}
		default:
			// 获取失败，说明有人正在使用。
			// 这种情况下我们不能访问 session.UpdatedAt() 以避免数据竞态。
			// 既然它正在被使用，它肯定不是目前最老且需要清理的闲置会话。
		}
	}

	// 按时间从旧到新排序（最老的在最后，方便使用 slice 操作）
	// candidates 中的所有会话目前都被本协程锁定，因此访问其 UpdatedAt 是安全的
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].session.UpdatedAt().After(candidates[j].session.UpdatedAt())
	})

	// 只要总数超标且还有候选会话，就从最老的开始删
	for len(r.sessions) > minRetainedSessions && len(candidates) > 0 {
		var oldest = candidates[len(candidates)-1]
		candidates = candidates[:len(candidates)-1]
		delete(r.sessions, oldest.session.ID())
		// 虽然会话被删除了，但必须放回 token
		// 这样如果有人刚好在 Acquire 中拿到了该 ownership 引用，他们可以正常结束而不是永久阻塞
		oldest.token <- struct{}{}
	}

	// 对于没有被删除的候选会话，必须放回 token
	for _, ownership := range candidates {
		updatedAt := ownership.session.UpdatedAt()
		if oldestActiveTime.IsZero() || updatedAt.Before(oldestActiveTime) {
			oldestActiveTime = updatedAt
		}
		ownership.token <- struct{}{}
	}

	// 计算下一次清理时间
	if !oldestActiveTime.IsZero() {
		r.nextCleanupTime = oldestActiveTime.Add(maxSessionIdleTime)
	} else {
		r.nextCleanupTime = time.Time{}
	}
}

var _ session.Repository = (*SessionRepository)(nil)
