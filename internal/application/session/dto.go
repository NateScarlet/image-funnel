package session

import (
	"time"

	appimage "main/internal/application/image"
	"main/internal/scalar"
)

type SessionDTO struct {
	ID           scalar.ID
	Directory    string
	Filter       *appimage.ImageFilters
	TargetKeep   int
	Status       Status
	Stats        *StatsDTO
	CreatedAt    time.Time
	UpdatedAt    time.Time
	CanCommit    bool
	CanUndo      bool
	CurrentImage *appimage.ImageDTO
	QueueStatus  *QueueStatusDTO
}

type StatsDTO struct {
	Total     int
	Processed int
	Kept      int
	Reviewed  int
	Rejected  int
	Remaining int
}

type QueueStatusDTO struct {
	CurrentIndex int
	TotalImages  int
	CurrentImage *appimage.ImageDTO
	Progress     float64
}

type Action string

const (
	ActionKeep    Action = "KEEP"
	ActionPending Action = "PENDING"
	ActionReject  Action = "REJECT"
)

type WriteActions struct {
	KeepRating    int
	PendingRating int
	RejectRating  int
}

type Status string

const (
	StatusInitializing Status = "INITIALIZING"
	StatusActive       Status = "ACTIVE"
	StatusPaused       Status = "PAUSED"
	StatusCompleted    Status = "COMPLETED"
	StatusCommitting   Status = "COMMITTING"
	StatusError        Status = "ERROR"
)
