package inmem

import (
	"sync"

	"main/internal/domain/session"
)

type SessionRepository struct {
	sessions map[string]*session.Session
	mu       sync.RWMutex
}

func NewSessionRepository() *SessionRepository {
	return &SessionRepository{
		sessions: make(map[string]*session.Session),
	}
}

func (r *SessionRepository) Save(sess *session.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[sess.ID()] = sess
	return nil
}

func (r *SessionRepository) FindByID(id string) (*session.Session, error) {
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

func (r *SessionRepository) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, id)
	return nil
}

var _ session.Repository = (*SessionRepository)(nil)
