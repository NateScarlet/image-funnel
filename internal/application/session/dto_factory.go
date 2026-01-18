package session

import (
	"main/internal/domain/session"
)

type SessionDTOFactory struct {
	urlSigner URLSigner
}

func NewSessionDTOFactory(urlSigner URLSigner) *SessionDTOFactory {
	return &SessionDTOFactory{
		urlSigner: urlSigner,
	}
}

func (f *SessionDTOFactory) New(sess *session.Session) (*SessionDTO, error) {
	imageDTOFactory := NewImageDTOFactory(f.urlSigner)
	statsDTOFactory := NewStatsDTOFactory()
	queueStatusDTOFactory := NewQueueStatusDTOFactory(f.urlSigner)

	stats, err := statsDTOFactory.New(sess.Stats())
	if err != nil {
		return nil, err
	}

	queueStatus, err := queueStatusDTOFactory.New(sess)
	if err != nil {
		return nil, err
	}

	var currentImage *ImageDTO
	if img := sess.CurrentImage(); img != nil {
		currentImage, err = imageDTOFactory.New(img)
		if err != nil {
			return nil, err
		}
	}

	return &SessionDTO{
		ID:           sess.ID(),
		Directory:    sess.Directory(),
		Filter:       toDTOFilter(sess.Filter()),
		TargetKeep:   sess.TargetKeep(),
		Status:       Status(sess.Status()),
		Stats:        stats,
		CreatedAt:    sess.CreatedAt(),
		UpdatedAt:    sess.UpdatedAt(),
		CanCommit:    sess.CanCommit(),
		CanUndo:      sess.CanUndo(),
		CurrentImage: currentImage,
		QueueStatus:  queueStatus,
	}, nil
}

type ImageDTOFactory struct {
	urlSigner URLSigner
}

func NewImageDTOFactory(urlSigner URLSigner) *ImageDTOFactory {
	return &ImageDTOFactory{
		urlSigner: urlSigner,
	}
}

func (f *ImageDTOFactory) New(img *session.Image) (*ImageDTO, error) {
	url, _ := f.urlSigner.GenerateSignedURL(img.Path())
	return &ImageDTO{
		ID:            img.ID(),
		Filename:      img.Filename(),
		Size:          img.Size(),
		URL:           url,
		ModTime:       img.ModTime(),
		CurrentRating: img.Rating(),
		XMPExists:     img.XMPExists(),
	}, nil
}

type StatsDTOFactory struct{}

func NewStatsDTOFactory() *StatsDTOFactory {
	return &StatsDTOFactory{}
}

func (f *StatsDTOFactory) New(stats *session.Stats) (*StatsDTO, error) {
	return &StatsDTO{
		Total:     stats.Total(),
		Processed: stats.Processed(),
		Kept:      stats.Kept(),
		Reviewed:  stats.Reviewed(),
		Rejected:  stats.Rejected(),
		Remaining: stats.Remaining(),
	}, nil
}

type QueueStatusDTOFactory struct {
	urlSigner URLSigner
}

func NewQueueStatusDTOFactory(urlSigner URLSigner) *QueueStatusDTOFactory {
	return &QueueStatusDTOFactory{
		urlSigner: urlSigner,
	}
}

func (f *QueueStatusDTOFactory) New(sess *session.Session) (*QueueStatusDTO, error) {
	imageDTOFactory := NewImageDTOFactory(f.urlSigner)
	stats := sess.Stats()
	progress := float64(0)
	if stats.Total() > 0 {
		progress = float64(stats.Processed()) / float64(stats.Total()) * 100
	}

	var currentImage *ImageDTO
	if img := sess.CurrentImage(); img != nil {
		var err error
		currentImage, err = imageDTOFactory.New(img)
		if err != nil {
			return nil, err
		}
	}

	return &QueueStatusDTO{
		CurrentIndex: sess.CurrentIndex(),
		TotalImages:  stats.Total(),
		CurrentImage: currentImage,
		Progress:     progress,
	}, nil
}
