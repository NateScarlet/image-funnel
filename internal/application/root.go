package application

import (
	"main/internal/application/device"
	"main/internal/application/directory"
	appimage "main/internal/application/image"
	"main/internal/application/memo"
	"main/internal/application/pairing"
	"main/internal/application/session"
)

type sessionHandler = session.Handler
type directoryHandler = directory.Handler
type memoHandler = memo.Handler
type imageHandler = appimage.Handler
type deviceHandler = device.Handler
type pairingHandler = pairing.Handler

// Root 直接嵌入了Handler，可以使用所有Handler方法
// 所有方法通过嵌入的Handler直接访问，不允许在Root结构体上重新声明
type Root struct {
	*sessionHandler
	*directoryHandler
	*memoHandler
	*imageHandler
	*deviceHandler
	*pairingHandler
}

func NewRoot(
	sessionHandler *session.Handler,
	directoryHandler *directory.Handler,
	memoHandler *memo.Handler,
	imageHandler *appimage.Handler,
	deviceHandler *device.Handler,
	pairingHandler *pairing.Handler,
) *Root {
	return &Root{
		sessionHandler:   sessionHandler,
		directoryHandler: directoryHandler,
		memoHandler:      memoHandler,
		imageHandler:     imageHandler,
		deviceHandler:    deviceHandler,
		pairingHandler:   pairingHandler,
	}
}
