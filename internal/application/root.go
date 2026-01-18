package application

import (
	"context"
	"iter"
	"main/internal/application/directory"
	"main/internal/application/session"
)

type sessionHandler = session.Handler
type directoryHandler = directory.Handler

type Root struct {
	*sessionHandler
	*directoryHandler
}

func NewRoot(
	sessionHandler *session.Handler,
	directoryHandler *directory.Handler,
) *Root {
	return &Root{
		sessionHandler:   sessionHandler,
		directoryHandler: directoryHandler,
	}
}

func (r *Root) SubscribeSession(ctx context.Context) iter.Seq2[*session.SessionDTO, error] {
	return r.sessionHandler.SubscribeSession(ctx)
}
