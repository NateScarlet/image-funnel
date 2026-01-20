package session

import "main/internal/scalar"

type Repository interface {
	Save(session *Session) error
	Get(id scalar.ID) (*Session, error)
	FindAll() ([]*Session, error)
	Delete(id scalar.ID) error
}
