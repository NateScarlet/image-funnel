package image

import (
	"time"

	"main/internal/scalar"
)

type ImageFilters struct {
	Rating []int
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
