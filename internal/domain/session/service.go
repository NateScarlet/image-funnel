package session

import (
	"main/internal/domain/image"
	"main/internal/domain/metadata"
	"main/internal/shared"
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
	session.status = shared.SessionStatusCommitting

	var errors []error
	success := 0

	for _, img := range session.Images() {
		action := session.GetAction(img.ID())

		var rating int
		switch action {
		case shared.ImageActionKeep:
			rating = writeActions.keepRating
		case shared.ImageActionPending:
			rating = writeActions.pendingRating
		case shared.ImageActionReject:
			rating = writeActions.rejectRating
		}
		if rating == img.Rating() {
			continue
		}

		xmpData := metadata.NewXMPData(rating, action.String(), time.Now(), "")

		if err := s.metadataRepo.Write(img.Path(), xmpData); err != nil {
			errors = append(errors, err)
			continue
		}
		success++
	}

	session.status = shared.SessionStatusCompleted

	return success, errors
}

// UpdateSessionSettings 更新会话的设置配置
func (s *Service) UpdateSessionSettings(session *Session, targetKeep *int, filter *image.ImageFilters) error {
	if targetKeep != nil {
		if err := session.UpdateTargetKeep(*targetKeep); err != nil {
			return err
		}
	}

	if filter != nil {
		if err := session.UpdateFilter(filter); err != nil {
			return err
		}
	}

	return nil
}
