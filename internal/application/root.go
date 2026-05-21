package application

import (
	"main/internal/application/directory"
	appimage "main/internal/application/image"
	"main/internal/application/memo"
	"main/internal/application/session"
)

type sessionHandler = session.Handler
type directoryHandler = directory.Handler
type memoHandler = memo.Handler
type imageHandler = appimage.Handler

// Root 直接嵌入了Handler，可以使用所有Handler方法
// 所有方法通过嵌入的Handler直接访问，不允许在Root结构体上重新声明
type Root struct {
	*sessionHandler
	*directoryHandler
	*memoHandler
	*imageHandler
}

func NewRoot(
	sessionHandler *session.Handler,
	directoryHandler *directory.Handler,
	memoHandler *memo.Handler,
	imageHandler *appimage.Handler,
) *Root {
	return &Root{
		sessionHandler:   sessionHandler,
		directoryHandler: directoryHandler,
		memoHandler:      memoHandler,
		imageHandler:     imageHandler,
	}
}
