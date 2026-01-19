package session

import (
	appimage "main/internal/application/image"
	"main/internal/domain/session"
	"main/internal/shared"
)

type SessionDTOFactory struct {
	urlSigner appimage.URLSigner
}

func NewSessionDTOFactory(urlSigner appimage.URLSigner) *SessionDTOFactory {
	return &SessionDTOFactory{
		urlSigner: urlSigner,
	}
}

func (f *SessionDTOFactory) New(sess *session.Session) (*shared.SessionDTO, error) {
	imageDTOFactory := appimage.NewImageDTOFactory(f.urlSigner)
	statsDTOFactory := NewStatsDTOFactory()
	queueStatusDTOFactory := NewQueueStatusDTOFactory(f.urlSigner)

	// 只计算一次统计信息
	sessionStats := sess.Stats()

	// 使用计算好的统计信息创建 StatsDTO
	stats, err := statsDTOFactory.New(sessionStats)
	if err != nil {
		return nil, err
	}

	// 使用计算好的统计信息创建 QueueStatusDTO
	queueStatus, err := queueStatusDTOFactory.NewWithStats(sess, sessionStats)
	if err != nil {
		return nil, err
	}

	var currentImage *shared.ImageDTO
	if img := sess.CurrentImage(); img != nil {
		currentImage, err = imageDTOFactory.New(img)
		if err != nil {
			return nil, err
		}
	}

	return &shared.SessionDTO{
		ID:           sess.ID(),
		Directory:    sess.Directory(),
		Filter:       sess.Filter(),
		TargetKeep:   sess.TargetKeep(),
		Status:       sessionStats.Status(),
		Stats:        stats,
		CreatedAt:    sess.CreatedAt(),
		UpdatedAt:    sess.UpdatedAt(),
		CanCommit:    sess.CanCommit(),
		CanUndo:      sess.CanUndo(),
		CurrentImage: currentImage,
		QueueStatus:  queueStatus,
	}, nil
}

type StatsDTOFactory struct{}

func NewStatsDTOFactory() *StatsDTOFactory {
	return &StatsDTOFactory{}
}

func (f *StatsDTOFactory) New(stats *session.Stats) (*shared.StatsDTO, error) {
	return &shared.StatsDTO{
		Total:     stats.Total(),
		Processed: stats.Processed(),
		Kept:      stats.Kept(),
		Reviewed:  stats.Reviewed(),
		Rejected:  stats.Rejected(),
		Remaining: stats.Remaining(),
	}, nil
}

type QueueStatusDTOFactory struct {
	urlSigner appimage.URLSigner
}

func NewQueueStatusDTOFactory(urlSigner appimage.URLSigner) *QueueStatusDTOFactory {
	return &QueueStatusDTOFactory{
		urlSigner: urlSigner,
	}
}

func (f *QueueStatusDTOFactory) New(sess *session.Session) (*shared.QueueStatusDTO, error) {
	stats := sess.Stats()
	return f.NewWithStats(sess, stats)
}

func (f *QueueStatusDTOFactory) NewWithStats(sess *session.Session, stats *session.Stats) (*shared.QueueStatusDTO, error) {
	imageDTOFactory := appimage.NewImageDTOFactory(f.urlSigner)
	progress := float64(0)
	if stats.Total() > 0 {
		progress = float64(stats.Processed()) / float64(stats.Total()) * 100
	}

	var currentImage *shared.ImageDTO
	if img := sess.CurrentImage(); img != nil {
		var err error
		currentImage, err = imageDTOFactory.New(img)
		if err != nil {
			return nil, err
		}
	}

	return &shared.QueueStatusDTO{
		CurrentIndex: sess.CurrentIndex(),
		TotalImages:  stats.Total(),
		CurrentImage: currentImage,
		Progress:     progress,
	}, nil
}
