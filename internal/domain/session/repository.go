package session

import (
	"iter"
	"main/internal/scalar"
)

type Repository interface {
	Save(session *Session) error
	Get(id scalar.ID) (*Session, error)
	FindByDirectory(directoryID scalar.ID) iter.Seq2[*Session, error]

	FindAll() ([]*Session, error)
	Delete(id scalar.ID) error
}
