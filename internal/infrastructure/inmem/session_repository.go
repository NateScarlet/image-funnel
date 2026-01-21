package inmem

import (
	"sort"
	"sync"
	"time"

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
	sessions map[scalar.ID]*session.Session
	mu       sync.RWMutex
}

func NewSessionRepository() *SessionRepository {
	return &SessionRepository{
		sessions: make(map[scalar.ID]*session.Session),
	}
}

func (r *SessionRepository) Save(sess *session.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[sess.ID()] = sess

	// 触发清理机制
	r.cleanup()
	return nil
}

func (r *SessionRepository) Get(id scalar.ID) (*session.Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	sess, exists := r.sessions[id]
	if !exists {
		return nil, session.ErrSessionNotFound
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

func (r *SessionRepository) Delete(id scalar.ID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, id)
	return nil
}

// cleanup 清理长时间未更新的会话
// 注意：此方法必须在持有写锁的情况下调用
func (r *SessionRepository) cleanup() {
	total := len(r.sessions)
	if total <= minRetainedSessions {
		return
	}

	threshold := time.Now().Add(-maxSessionIdleTime)

	var expired []*session.Session
	for _, s := range r.sessions {
		if !s.UpdatedAt().After(threshold) {
			expired = append(expired, s)
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
}

var _ session.Repository = (*SessionRepository)(nil)
