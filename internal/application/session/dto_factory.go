package session

import (
	"main/internal/domain/session"
	"main/internal/scalar"
	"main/internal/shared"
)

type DTOFactory struct {
}

func NewDTOFactory() *DTOFactory {
	return &DTOFactory{}
}

func (f *DTOFactory) New(sess *session.Session) (*shared.SessionDTO, error) {

	sessionStats := sess.Stats()

	var currentImageID scalar.ID
	if img := sess.CurrentImage(); img != nil {
		currentImageID = img.ID()
	}

	return &shared.SessionDTO{
		ID:                  sess.ID(),
		DirectoryID:         sess.DirectoryID(),
		Filter:              sess.Filter(),
		TargetKeep:          sess.TargetKeep(),
		Stats:               sessionStats,
		CreatedAt:           sess.CreatedAt(),
		UpdatedAt:           sess.UpdatedAt(),
		CanCommit:           sess.CanCommit(),
		CanUndo:             sess.CanUndo(),
		CurrentIndex:        sess.CurrentIndex(),
		CurrentSize:         sess.CurrentSize(),
		CurrentRound:        sess.CurrentRound(),
		CurrentImageID:      currentImageID,
		CurrentRoundActions: sess.CurrentRoundActions(),
	}, nil
}
