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

	// 只计算一次统计信息
	sessionStats := sess.Stats()

	// 使用计算好的统计信息创建 StatsDTO
	stats, err := statsDTOFactory.New(sessionStats)
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
		DirectoryID:  sess.DirectoryID(),
		Filter:       sess.Filter(),
		TargetKeep:   sess.TargetKeep(),
		Stats:        stats,
		CreatedAt:    sess.CreatedAt(),
		UpdatedAt:    sess.UpdatedAt(),
		CanCommit:    sess.CanCommit(),
		CanUndo:      sess.CanUndo(),
		CurrentIndex: sess.CurrentIndex(),
		CurrentSize:  sess.CurrentSize(),
		CurrentImage: currentImage,
	}, nil
}

type StatsDTOFactory struct{}

func NewStatsDTOFactory() *StatsDTOFactory {
	return &StatsDTOFactory{}
}

func (f *StatsDTOFactory) New(stats *session.Stats) (*shared.StatsDTO, error) {
	return &shared.StatsDTO{
		Total:       stats.Total(),
		Kept:        stats.Kept(),
		Shelved:     stats.Shelved(),
		Rejected:    stats.Rejected(),
		Remaining:   stats.Remaining(),
		IsCompleted: stats.IsCompleted(),
	}, nil
}
