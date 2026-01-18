package session

import (
	"time"

	"main/internal/scalar"
)

type SessionDTO struct {
	ID           scalar.ID
	Directory    string
	Filter       *ImageFilters
	TargetKeep   int
	Status       Status
	Stats        *StatsDTO
	CreatedAt    time.Time
	UpdatedAt    time.Time
	CanCommit    bool
	CanUndo      bool
	CurrentImage *ImageDTO
	QueueStatus  *QueueStatusDTO
}

type ImageDTO struct {
	ID            scalar.ID
	Filename      string
	Size          int64
	URL           string
	ModTime       time.Time
	CurrentRating int
	XMPExists     bool
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
	CurrentImage *ImageDTO
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

type ImageFilters struct {
	Rating []int
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
