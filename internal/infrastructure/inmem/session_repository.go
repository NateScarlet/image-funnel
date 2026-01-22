package inmem

import (
	"iter"
	"slices"
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

type SessionRepository struct {
	sessions        map[scalar.ID]*session.Session
	dirIndex        map[scalar.ID][]scalar.ID
	mu              sync.RWMutex
	nextCleanupTime time.Time
}

func NewSessionRepository() *SessionRepository {
	return &SessionRepository{
		sessions: make(map[scalar.ID]*session.Session),
		dirIndex: make(map[scalar.ID][]scalar.ID),
	}
}

func (r *SessionRepository) Save(sess *session.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if update or new
	_, exists := r.sessions[sess.ID()]
	r.sessions[sess.ID()] = sess

	// Update directory index if new
	if !exists {
		dirID := sess.DirectoryID()
		// Simple append, assuming no duplicates because we check 'exists' in sessions
		r.dirIndex[dirID] = append(r.dirIndex[dirID], sess.ID())
	} else {
		// If directory ID could change, we would need to handle that, but typically it doesn't.
		// If it did, we'd need to remove from old and add to new.
		// For now assuming DirectoryID is immutable for a session as per domain logic usually.
	}

	// 触发清理机制
	r.cleanup()
	return nil
}

func (r *SessionRepository) Get(id scalar.ID) (*session.Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	sess, exists := r.sessions[id]
	if !exists {
		return nil, apperror.NewErrDocumentNotFound(id)
	}
	return sess, nil
}

func (r *SessionRepository) FindAll() ([]*session.Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*session.Session, 0, len(r.sessions))
	for _, sess := range r.sessions {
		result = append(result, sess)
	}
	return result, nil
}

func (r *SessionRepository) FindByDirectory(directoryID scalar.ID) iter.Seq2[*session.Session, error] {
	return func(yield func(*session.Session, error) bool) {
		r.mu.RLock()
		var s []*session.Session // 复制避免迭代过程中保存导致死锁
		for _, i := range r.dirIndex[directoryID] {
			s = append(s, r.sessions[i])
		}
		r.mu.RUnlock()

		for _, sess := range s {
			if !yield(sess, nil) {
				return
			}
		}
	}
}

func (r *SessionRepository) Delete(id scalar.ID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if sess, ok := r.sessions[id]; ok {
		dirID := sess.DirectoryID()
		if ids, ok := r.dirIndex[dirID]; ok {
			newIDs := slices.DeleteFunc(ids, func(e scalar.ID) bool {
				return e == id
			})
			if len(newIDs) == 0 {
				delete(r.dirIndex, dirID)
			} else {
				r.dirIndex[dirID] = newIDs
			}
		}
		delete(r.sessions, id)
	}
	return nil
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
		// 下一次清理至少要等到有新的会话加入，或者现有会话过期（虽然不会被删除）
		// 但为了简单，这里不设置具体时间，等待下次 Save 触发判断即可
		// 或者可以设置一个较长的间隔，防止在此期间频繁调用（虽然目前只在 Save 调用）
		return
	}

	threshold := now.Add(-maxSessionIdleTime)

	var expired []*session.Session
	var oldestActiveTime time.Time

	for _, s := range r.sessions {
		updatedAt := s.UpdatedAt()
		if !updatedAt.After(threshold) {
			expired = append(expired, s)
		} else {
			// 记录活跃会话中最早的更新时间，它将是下一个可能过期的会话
			if oldestActiveTime.IsZero() || updatedAt.Before(oldestActiveTime) {
				oldestActiveTime = updatedAt
			}
		}
	}

	// 按时间倒序排序（最新的在前），这样 slice 的末尾就是最老的
	sort.Slice(expired, func(i, j int) bool {
		return expired[i].UpdatedAt().After(expired[j].UpdatedAt())
	})

	// 只要总数超标且还有过期会话，就从最老的开始删
	for len(r.sessions) > minRetainedSessions && len(expired) > 0 {
		var oldest = expired[len(expired)-1]
		expired = expired[:len(expired)-1]
		delete(r.sessions, oldest.ID())
	}

	// 计算下一次清理时间
	// 如果还有剩余的会话（包括保留的过期会话和活跃会话）
	// 下一次清理时间应该是：最老的活跃会话变成过期的时间
	if !oldestActiveTime.IsZero() {
		r.nextCleanupTime = oldestActiveTime.Add(maxSessionIdleTime)
	} else {
		// 如果没有活跃会话了（全是保留的过期会话），则暂时不需要因为时间原因清理
		// 重置为一个较远的未来，直到有新会话进来更新状态
		r.nextCleanupTime = time.Time{}
	}
}

var _ session.Repository = (*SessionRepository)(nil)
