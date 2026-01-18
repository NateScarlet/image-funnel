//go:generate go tool github.com/99designs/gqlgen

package graph

import (
	"context"
	"fmt"
	_ "image/jpeg"
	_ "image/png"
	"time"

	"main/internal/pubsub"
	"main/internal/session"
	"main/internal/url"
)

type Resolver struct {
	SessionManager *session.Manager
	RootDir        string
	Signer         *url.Signer
	SessionTopic   pubsub.Topic[*Session]
	Version        string
}

func NewResolver(rootDir string, signer *url.Signer, version string) *Resolver {
	topic, _ := pubsub.NewInMemoryTopic[*Session]()
	return &Resolver{
		SessionManager: session.NewManager(),
		RootDir:        rootDir,
		Signer:         signer,
		SessionTopic:   topic,
		Version:        version,
	}
}

func (r *Resolver) Session(ctx context.Context, id string) (*Session, error) {
	sess, exists := r.SessionManager.Get(id)
	if !exists {
		return nil, fmt.Errorf("session not found")
	}
	return r.convertToGQLSession(sess), nil
}

func (r *Resolver) SessionUpdated(ctx context.Context, sessionID string) (<-chan *Session, error) {
	ch := make(chan *Session, 1)

	go func() {
		defer close(ch)

		for sess := range r.SessionTopic.Subscribe(ctx) {
			if sess.ID != sessionID {
				continue
			}

			select {
			case ch <- sess:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}

func (r *Resolver) convertToGQLImageFromSession(img *session.ImageInfo) *Image {
	rating := img.CurrentRating()
	url, err := r.Signer.GenerateSignedURL(img.Path())
	if err != nil {
		url = ""
	}
	return &Image{
		ID:            img.ID(),
		Filename:      img.Filename(),
		Size:          int(img.Size()),
		URL:           url,
		CurrentRating: &rating,
		XmpExists:     img.XMPExists(),
	}
}

func (r *Resolver) convertToGQLSession(s *session.Session) *Session {
	stats := s.Stats()
	currentImg := s.CurrentImage()
	var gqlCurrentImage *Image
	if currentImg != nil {
		gqlCurrentImage = r.convertToGQLImageFromSession(currentImg)
	}

	progress := float64(0)
	if stats.Total > 0 {
		progress = float64(stats.Processed) / float64(stats.Total) * 100
	}

	return &Session{
		ID:           s.ID,
		Directory:    s.Directory,
		Filter:       s.Filter,
		TargetKeep:   s.TargetKeep,
		Status:       SessionStatus(s.Status),
		Stats:        r.convertToGQLStats(&stats),
		CreatedAt:    s.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    s.UpdatedAt.Format(time.RFC3339),
		CanCommit:    s.CanCommit(),
		CanUndo:      s.CanUndo(),
		CurrentImage: gqlCurrentImage,
		QueueStatus: &QueueStatus{
			CurrentIndex: s.CurrentIdx,
			TotalImages:  stats.Total,
			CurrentImage: gqlCurrentImage,
			Progress:     progress,
		},
	}
}

func (r *Resolver) convertToGQLStats(stats *session.Stats) *SessionStats {
	return &SessionStats{
		Total:     stats.Total,
		Processed: stats.Processed,
		Kept:      stats.Kept,
		Reviewed:  stats.Reviewed,
		Rejected:  stats.Rejected,
		Remaining: stats.Remaining,
	}
}
