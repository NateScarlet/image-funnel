package session

type Repository interface {
	Save(session *Session) error
	FindByID(id string) (*Session, error)
	FindAll() ([]*Session, error)
	Delete(id string) error
}
