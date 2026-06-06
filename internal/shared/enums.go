package shared

import (
	"main/internal/enum"
)

type ImageActionMeta struct{}

var imageAction = enum.New[ImageActionMeta]()
var (
	ImageActionKeep   = imageAction.Define("KEEP")
	ImageActionShelve = imageAction.Define("SHELVE")
	ImageActionReject = imageAction.Define("REJECT")
)

type ImageAction = enum.Enum[ImageActionMeta]

type FileActionMeta struct{}

var fileAction = enum.New[FileActionMeta]()
var (
	FileActionCreate = fileAction.Define("CREATE")
	FileActionWrite  = fileAction.Define("WRITE")
	FileActionRemove = fileAction.Define("REMOVE")
	FileActionRename = fileAction.Define("RENAME")
)

type FileAction = enum.Enum[FileActionMeta]

type PairingRequestStatusMeta struct{}

var pairingRequestStatus = enum.New[PairingRequestStatusMeta]()
var (
	PairingRequestStatusPending  = pairingRequestStatus.Define("PENDING")
	PairingRequestStatusApproved = pairingRequestStatus.Define("APPROVED")
	PairingRequestStatusRejected = pairingRequestStatus.Define("REJECTED")
)

type PairingRequestStatus = enum.Enum[PairingRequestStatusMeta]
