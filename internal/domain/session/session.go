package session

import (
	"main/internal/domain/image"
	"main/internal/scalar"
	"main/internal/shared"
	"time"
)

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
	id         scalar.ID
	directory  string
	filter     *image.ImageFilters
	targetKeep int
	status     shared.SessionStatus
	createdAt  time.Time
	updatedAt  time.Time

	images     []*image.Image
	queue      []*image.Image
	currentIdx int
	undoStack  []UndoEntry
	actions    map[scalar.ID]shared.ImageAction

	roundHistory []RoundSnapshot
	currentRound int
}

type RoundSnapshot struct {
	queue      []*image.Image
	currentIdx int
	undoStack  []UndoEntry
}

func NewSession(id scalar.ID, directory string, filter *image.ImageFilters, targetKeep int, images []*image.Image) *Session {
	actions := make(map[scalar.ID]shared.ImageAction)
	for _, img := range images {
		actions[img.ID()] = shared.ImageActionPending
	}
	return &Session{
		id:           id,
		directory:    directory,
		filter:       filter,
		targetKeep:   targetKeep,
		status:       shared.SessionStatusActive,
		createdAt:    time.Now(),
		updatedAt:    time.Now(),
		images:       images,
		queue:        images,
		currentIdx:   0,
		undoStack:    make([]UndoEntry, 0),
		actions:      actions,
		roundHistory: make([]RoundSnapshot, 0),
		currentRound: 0,
	}
}

func (s *Session) ID() scalar.ID {
	return s.id
}

func (s *Session) Directory() string {
	return s.directory
}

func (s *Session) Filter() *image.ImageFilters {
	return s.filter
}

func (s *Session) TargetKeep() int {
	return s.targetKeep
}

func (s *Session) Status() shared.SessionStatus {
	return s.status
}

func (s *Session) CreatedAt() time.Time {
	return s.createdAt
}

func (s *Session) UpdatedAt() time.Time {
	return s.updatedAt
}

func (s *Session) CurrentImage() *image.Image {
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
		action := s.actions[img.ID()]
		switch action {
		case shared.ImageActionKeep:
			stats.kept++
		case shared.ImageActionPending:
			stats.reviewed++
		case shared.ImageActionReject:
			stats.rejected++
		}
	}

	stats.rejected += len(s.images) - len(s.queue)

	return &stats
}

func (s *Session) CanCommit() bool {
	if s.status == shared.SessionStatusCommitting || s.status == shared.SessionStatusError {
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

func (s *Session) MarkImage(imageID scalar.ID, action shared.ImageAction) error {
	if s.status != shared.SessionStatusActive {
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
		action:  s.actions[imageID],
	})

	s.actions[imageID] = action
	s.updatedAt = time.Now()

	s.currentIdx++

	if s.currentIdx >= len(s.queue) {
		if s.Stats().reviewed > 0 || s.Stats().kept > 0 {
			var newQueue []*image.Image
			for _, img := range s.queue {
				action := s.actions[img.ID()]
				if action == shared.ImageActionPending || action == shared.ImageActionKeep {
					newQueue = append(newQueue, img)
				}
			}
			if len(newQueue) > 0 {
				if len(newQueue) <= s.targetKeep {
					s.status = shared.SessionStatusCompleted
				} else {
					s.roundHistory = append(s.roundHistory, RoundSnapshot{
						queue:      s.queue,
						currentIdx: s.currentIdx,
						undoStack:  s.undoStack,
					})
					s.currentRound++
					s.queue = newQueue
					s.currentIdx = 0
					s.undoStack = make([]UndoEntry, 0)
				}
			} else {
				s.status = shared.SessionStatusCompleted
			}
		} else {
			s.status = shared.SessionStatusCompleted
		}
	}

	return nil
}

func (s *Session) Undo() error {
	if len(s.undoStack) == 0 {
		if len(s.roundHistory) == 0 {
			return ErrNothingToUndo
		}

		lastRound := s.roundHistory[len(s.roundHistory)-1]
		s.roundHistory = s.roundHistory[:len(s.roundHistory)-1]
		s.currentRound--
		s.queue = lastRound.queue
		s.currentIdx = lastRound.currentIdx
		s.undoStack = lastRound.undoStack
		s.status = shared.SessionStatusActive
		s.updatedAt = time.Now()
		return nil
	}

	lastEntry := s.undoStack[len(s.undoStack)-1]
	s.undoStack = s.undoStack[:len(s.undoStack)-1]

	for _, img := range s.images {
		if img.ID() == lastEntry.imageID {
			s.actions[img.ID()] = lastEntry.action

			s.currentIdx--
			s.status = shared.SessionStatusActive
			s.updatedAt = time.Now()
			return nil
		}
	}

	return ErrSessionNotFound
}

func (s *Session) Images() []*image.Image {
	return s.images
}

func (s *Session) GetAction(imageID scalar.ID) shared.ImageAction {
	if action, exists := s.actions[imageID]; exists {
		return action
	}
	return shared.ImageActionPending
}

func (s *Session) SetAction(imageID scalar.ID, action shared.ImageAction) {
	s.actions[imageID] = action
}

type UndoEntry struct {
	imageID scalar.ID
	action  shared.ImageAction
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
