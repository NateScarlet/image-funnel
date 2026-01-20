package inmem

import (
	"sync"

	"main/internal/domain/session"
	"main/internal/scalar"
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

var _ session.Repository = (*SessionRepository)(nil)
