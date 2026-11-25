package application

import (
	"main/internal/application/device"
	"main/internal/application/directory"
	"main/internal/application/hook"
	appimage "main/internal/application/image"
	"main/internal/application/note"
	"main/internal/application/notification"
	"main/internal/application/pairing"
	"main/internal/application/session"
)

type sessionHandler = session.Handler
type directoryHandler = directory.Handler
type noteHandler = note.Handler
type imageHandler = appimage.Handler
type deviceHandler = device.Handler
type pairingHandler = pairing.Handler
type hookHandler = hook.Handler
type notificationHandler = notification.Handler

// Root 直接嵌入了各个领域 Handler，可直接使用所有被提升的 Handler 方法。
// 【警告】所有领域的 Handler 方法全局不应该重名（例如列表查询应直接使用返回类型命名，如 Devices/Hooks 替代通用的 List），
// 否则会导致 Root 结构体嵌入时发生方法名冲突，破坏直接提升方法的设计。
type Root struct {
	*sessionHandler
	*directoryHandler
	*noteHandler
	*imageHandler
	*deviceHandler
	*pairingHandler
	*hookHandler
	*notificationHandler
}

func NewRoot(
	sessionHandler *session.Handler,
	directoryHandler *directory.Handler,
	noteHandler *note.Handler,
	imageHandler *appimage.Handler,
	deviceHandler *device.Handler,
	pairingHandler *pairing.Handler,
	hookHandler *hook.Handler,
	notificationHandler *notification.Handler,
) *Root {
	return &Root{
		sessionHandler:      sessionHandler,
		directoryHandler:    directoryHandler,
		noteHandler:         noteHandler,
		imageHandler:        imageHandler,
		deviceHandler:       deviceHandler,
		pairingHandler:      pairingHandler,
		hookHandler:         hookHandler,
		notificationHandler: notificationHandler,
	}
}
