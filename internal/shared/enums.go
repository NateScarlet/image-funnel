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
