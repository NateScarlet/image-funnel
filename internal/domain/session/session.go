package session

import (
	"main/internal/domain/metadata"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusInitializing Status = "INITIALIZING"
	StatusActive       Status = "ACTIVE"
	StatusPaused       Status = "PAUSED"
	StatusCompleted    Status = "COMPLETED"
	StatusCommitting   Status = "COMMITTING"
	StatusError        Status = "ERROR"
)

type Action string

const (
	ActionKeep    Action = "KEEP"
	ActionPending Action = "PENDING"
	ActionReject  Action = "REJECT"
)

type ImageFilters struct {
	rating []int
}

func NewImageFilters(rating []int) *ImageFilters {
	return &ImageFilters{
		rating: rating,
	}
}

func (f *ImageFilters) Rating() []int {
	if f == nil {
		return nil
	}
	return f.rating
}

type WriteActions struct {
	keepRating    int
	pendingRating int
	rejectRating  int
}

func NewWriteActions(keepRating, pendingRating, rejectRating int) *WriteActions {
	return &WriteActions{
		keepRating:    keepRating,
		pendingRating: pendingRating,
		rejectRating:  rejectRating,
	}
}

func (a *WriteActions) KeepRating() int {
	return a.keepRating
}

func (a *WriteActions) PendingRating() int {
	return a.pendingRating
}

func (a *WriteActions) RejectRating() int {
	return a.rejectRating
}

type Stats struct {
	total     int
	processed int
	kept      int
	reviewed  int
	rejected  int
	remaining int
}

func (s *Stats) Total() int {
	return s.total
}

func (s *Stats) Processed() int {
	return s.processed
}

func (s *Stats) Kept() int {
	return s.kept
}

func (s *Stats) Reviewed() int {
	return s.reviewed
}

func (s *Stats) Rejected() int {
	return s.rejected
}

func (s *Stats) Remaining() int {
	return s.remaining
}

type Session struct {
	id         string
	directory  string
	filter     *ImageFilters
	targetKeep int
	status     Status
	createdAt  time.Time
	updatedAt  time.Time

	images     []*Image
	queue      []*Image
	currentIdx int
	undoStack  []UndoEntry
}

func NewSession(directory string, filter *ImageFilters, targetKeep int, images []*Image) *Session {
	return &Session{
		id:         generateID(),
		directory:  directory,
		filter:     filter,
		targetKeep: targetKeep,
		status:     StatusActive,
		createdAt:  time.Now(),
		updatedAt:  time.Now(),
		images:     images,
		queue:      images,
		currentIdx: 0,
		undoStack:  make([]UndoEntry, 0),
	}
}

func (s *Session) ID() string {
	return s.id
}

func (s *Session) Directory() string {
	return s.directory
}

func (s *Session) Filter() *ImageFilters {
	return s.filter
}

func (s *Session) TargetKeep() int {
	return s.targetKeep
}

func (s *Session) Status() Status {
	return s.status
}

func (s *Session) CreatedAt() time.Time {
	return s.createdAt
}

func (s *Session) UpdatedAt() time.Time {
	return s.updatedAt
}

func (s *Session) CurrentImage() *Image {
	if s.currentIdx < len(s.queue) {
		return s.queue[s.currentIdx]
	}
	return nil
}

func (s *Session) CurrentIndex() int {
	return s.currentIdx
}

func (s *Session) Stats() *Stats {
	var stats Stats
	stats.total = len(s.queue)
	stats.processed = s.currentIdx
	stats.remaining = len(s.queue) - s.currentIdx

	for i := 0; i < s.currentIdx; i++ {
		img := s.queue[i]
		action := img.Action()
		switch action {
		case ActionKeep:
			stats.kept++
		case ActionPending:
			stats.reviewed++
		case ActionReject:
			stats.rejected++
		}
	}

	stats.rejected += len(s.images) - len(s.queue)

	return &stats
}

func (s *Session) CanCommit() bool {
	if s.status == StatusCommitting || s.status == StatusError {
		return false
	}

	stats := s.Stats()
	if stats.processed > 0 {
		return true
	}

	return len(s.images) > len(s.queue)
}

func (s *Session) CanUndo() bool {
	return len(s.undoStack) > 0
}

func (s *Session) MarkImage(imageID string, action Action) error {
	if s.status != StatusActive {
		return ErrSessionNotActive
	}

	if s.currentIdx >= len(s.queue) {
		return ErrNoMoreImages
	}

	currentImage := s.queue[s.currentIdx]
	if currentImage.ID() != imageID {
		found := false
		for i, img := range s.queue {
			if img.ID() == imageID {
				currentImage = img
				s.currentIdx = i
				found = true
				break
			}
		}
		if !found {
			return ErrSessionNotFound
		}
	}

	s.undoStack = append(s.undoStack, UndoEntry{
		imageID: imageID,
		action:  currentImage.Action(),
	})

	currentImage.SetAction(action)
	s.updatedAt = time.Now()

	s.currentIdx++

	if s.currentIdx >= len(s.queue) {
		if s.Stats().reviewed > 0 || s.Stats().kept > 0 {
			var newQueue []*Image
			for _, img := range s.queue {
				action := img.Action()
				if action == ActionPending || action == ActionKeep {
					newQueue = append(newQueue, img)
				}
			}
			if len(newQueue) > 0 {
				if len(newQueue) <= s.targetKeep {
					s.status = StatusCompleted
				} else {
					s.queue = newQueue
					s.currentIdx = 0
					s.undoStack = make([]UndoEntry, 0)
				}
			} else {
				s.status = StatusCompleted
			}
		} else {
			s.status = StatusCompleted
		}
	}

	return nil
}

func (s *Session) Undo() error {
	if len(s.undoStack) == 0 {
		return ErrNothingToUndo
	}

	lastEntry := s.undoStack[len(s.undoStack)-1]
	s.undoStack = s.undoStack[:len(s.undoStack)-1]

	for _, img := range s.images {
		if img.ID() == lastEntry.imageID {
			img.SetAction(lastEntry.action)

			s.currentIdx--
			s.status = StatusActive
			s.updatedAt = time.Now()
			return nil
		}
	}

	return ErrSessionNotFound
}

func (s *Session) Commit(writeActions *WriteActions) (int, []error) {
	s.status = StatusCommitting

	var errors []error
	success := 0

	for _, img := range s.images {
		action := img.Action()

		var rating int
		switch action {
		case ActionKeep:
			rating = writeActions.keepRating
		case ActionPending:
			rating = writeActions.pendingRating
		case ActionReject:
			rating = writeActions.rejectRating
		}
		if rating == 0 && !img.XMPExists() {
			continue
		}

		xmpData := metadata.NewXMPData(rating, string(action), s.id, time.Now(), "")

		if err := WriteXMP(img.Path(), xmpData); err != nil {
			errors = append(errors, err)
			continue
		}
		success++
	}

	s.status = StatusCompleted

	return success, errors
}

func (s *Session) Images() []*Image {
	return s.images
}

type Image struct {
	imageID       string
	filename      string
	imagePath     string
	size          int64
	currentRating int
	xmpExists     bool
	action        Action
}

func NewImage(id, filename, path string, size int64, currentRating int, xmpExists bool) *Image {
	return &Image{
		imageID:       id,
		filename:      filename,
		imagePath:     path,
		size:          size,
		currentRating: currentRating,
		xmpExists:     xmpExists,
		action:        ActionPending,
	}
}

func (i *Image) ID() string {
	return i.imageID
}

func (i *Image) Filename() string {
	return i.filename
}

func (i *Image) Path() string {
	return i.imagePath
}

func (i *Image) Size() int64 {
	return i.size
}

func (i *Image) Rating() int {
	return i.currentRating
}

func (i *Image) XMPExists() bool {
	return i.xmpExists
}

func (i *Image) Action() Action {
	if i.action == "" {
		return ActionPending
	}
	return i.action
}

func (i *Image) SetAction(action Action) {
	i.action = action
}

type UndoEntry struct {
	imageID string
	action  Action
}

func generateID() string {
	return uuid.New().String()
}

var (
	ErrSessionNotActive = &SessionError{message: "session is not active"}
	ErrNoMoreImages     = &SessionError{message: "no more images"}
	ErrSessionNotFound  = &SessionError{message: "session not found"}
	ErrNothingToUndo    = &SessionError{message: "nothing to undo"}
)

type SessionError struct {
	message string
}

func (e *SessionError) Error() string {
	return e.message
}
