package session

import (
	"main/internal/domain/metadata"
	"time"
)

type Service struct {
	metadataRepo metadata.Repository
}

func NewService(metadataRepo metadata.Repository) *Service {
	return &Service{
		metadataRepo: metadataRepo,
	}
}

func (s *Service) Commit(session *Session, writeActions *WriteActions) (int, []error) {
	session.status = StatusCommitting

	var errors []error
	success := 0

	for _, img := range session.images {
		action := img.Action()

		var rating int
		switch action {
		case ActionKeep:
			rating = writeActions.keepRating
		case ActionPending:
			rating = writeActions.pendingRating
		case ActionReject:
			rating = writeActions.rejectRating
		}
		if rating == 0 && !img.XMPExists() {
			continue
		}

		xmpData := metadata.NewXMPData(rating, string(action), session.id, time.Now(), "")

		if err := s.metadataRepo.Write(img.Path(), xmpData); err != nil {
			errors = append(errors, err)
			continue
		}
		success++
	}

	session.status = StatusCompleted

	return success, errors
}
