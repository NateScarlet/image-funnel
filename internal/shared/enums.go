package shared

import (
	"main/internal/enum"
)

type ImageActionMeta struct{}

var imageAction = enum.New[ImageActionMeta]()
var (
	ImageActionKeep    = imageAction.Define("KEEP")
	ImageActionPending = imageAction.Define("PENDING")
	ImageActionReject  = imageAction.Define("REJECT")
)

type ImageAction = enum.Enum[ImageActionMeta]

type SessionStatusMeta struct{}

var sessionStatus = enum.New[SessionStatusMeta]()
var (
	SessionStatusInitializing = sessionStatus.Define("INITIALIZING")
	SessionStatusActive       = sessionStatus.Define("ACTIVE")
	SessionStatusPaused       = sessionStatus.Define("PAUSED")
	SessionStatusCompleted    = sessionStatus.Define("COMPLETED")
	SessionStatusCommitting   = sessionStatus.Define("COMMITTING")
	SessionStatusError        = sessionStatus.Define("ERROR")
)

type SessionStatus = enum.Enum[SessionStatusMeta]
