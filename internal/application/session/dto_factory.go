package session

import (
	appimage "main/internal/application/image"
	"main/internal/domain/session"
	"main/internal/shared"
)

type SessionDTOFactory struct {
	imageDTOFactory *appimage.ImageDTOFactory
}

func NewSessionDTOFactory(imageDTOFactory *appimage.ImageDTOFactory) *SessionDTOFactory {
	return &SessionDTOFactory{
		imageDTOFactory: imageDTOFactory,
	}
}

func (f *SessionDTOFactory) New(sess *session.Session) (*shared.SessionDTO, error) {

	// 只计算一次统计信息
	sessionStats := sess.Stats()

	var currentImage *shared.ImageDTO
	var err error
	if img := sess.CurrentImage(); img != nil {
		currentImage, err = f.imageDTOFactory.New(img)
		if err != nil {
			return nil, err
		}
	}

	return &shared.SessionDTO{
		ID:           sess.ID(),
		DirectoryID:  sess.DirectoryID(),
		Filter:       sess.Filter(),
		TargetKeep:   sess.TargetKeep(),
		Stats:        sessionStats,
		CreatedAt:    sess.CreatedAt(),
		UpdatedAt:    sess.UpdatedAt(),
		CanCommit:    sess.CanCommit(),
		CanUndo:      sess.CanUndo(),
		CurrentIndex: sess.CurrentIndex(),
		CurrentSize:  sess.CurrentSize(),
		CurrentRound: sess.CurrentRound(),
		CurrentImage: currentImage,
		CurrentRoundActions: sess.CurrentRoundActions(),
	}, nil
}
