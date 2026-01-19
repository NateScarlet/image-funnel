package application

import (
	"main/internal/application/directory"
	"main/internal/application/session"
)

type sessionHandler = session.Handler
type directoryHandler = directory.Handler

// Root 直接嵌入了Handler，可以使用所有Handler方法
// 所有方法通过嵌入的Handler直接访问，不允许在Root结构体上重新声明
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
