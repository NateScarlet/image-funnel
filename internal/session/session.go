package session

import (
	"fmt"
	"sync"
	"time"

	"main/internal/scanner"
	"main/internal/xmp"
)

type ImageFilters struct {
	Rating []int
}

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

type Action string

const (
	ActionKeep    Action = "KEEP"
	ActionPending Action = "PENDING"
	ActionReject  Action = "REJECT"
)

type Stats struct {
	Total     int
	Processed int
	Kept      int
	Reviewed  int
	Rejected  int
	Remaining int
}

type Session struct {
	ID         string
	Directory  string
	Filter     *ImageFilters
	TargetKeep int
	Status     Status
	CreatedAt  time.Time
	UpdatedAt  time.Time

	images     []*ImageInfo
	queue      []*ImageInfo
	CurrentIdx int
	undoStack  []UndoEntry
	mu         sync.RWMutex
}

func (s *Session) CurrentImage() *ImageInfo {
	if s.CurrentIdx < len(s.queue) {
		return s.queue[s.CurrentIdx]
	}
	return nil
}

func (s *Session) Stats() Stats {
	var stats Stats
	stats.Total = len(s.queue)
	stats.Processed = s.CurrentIdx
	stats.Remaining = len(s.queue) - s.CurrentIdx

	for i := 0; i < s.CurrentIdx; i++ {
		img := s.queue[i]
		action := img.Action()
		switch action {
		case ActionKeep:
			stats.Kept++
		case ActionPending:
			stats.Reviewed++
		case ActionReject:
			stats.Rejected++
		}
	}

	stats.Rejected += len(s.images) - len(s.queue)

	return stats
}

func (s *Session) CanCommit() bool {
	if s.Status == StatusCommitting || s.Status == StatusError {
		return false
	}

	stats := s.Stats()
	if stats.Processed > 0 {
		return true
	}

	return len(s.images) > len(s.queue)
}

func (s *Session) CanUndo() bool {
	return len(s.undoStack) > 0
}

func (s *Session) Images() []*ImageInfo {
	return s.images
}

type ImageInfo struct {
	id            string
	filename      string
	path          string
	size          int64
	currentRating int
	xmpExists     bool
	action        Action
}

func (i *ImageInfo) ID() string {
	return i.id
}

func (i *ImageInfo) Filename() string {
	return i.filename
}

func (i *ImageInfo) Path() string {
	return i.path
}

func (i *ImageInfo) Size() int64 {
	return i.size
}

func (i *ImageInfo) CurrentRating() int {
	return i.currentRating
}

func (i *ImageInfo) XMPExists() bool {
	return i.xmpExists
}

func (i *ImageInfo) Action() Action {
	if i.action == "" {
		return ActionPending
	}
	return i.action
}

func (i *ImageInfo) SetAction(action Action) {
	i.action = action
}

type UndoEntry struct {
	ImageID string
	Action  Action
}

type Manager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
	}
}

func (m *Manager) Create(dirPath string, filter *ImageFilters, targetKeep int) (*Session, error) {
	scanner := scanner.NewScanner(dirPath)
	images, err := scanner.Scan()
	if err != nil {
		return nil, fmt.Errorf("failed to scan directory: %w", err)
	}

	var queue []*ImageInfo
	for _, img := range images {
		if contains(filter.Rating, img.CurrentRating) || img.CurrentRating == 0 {
			queue = append(queue, convertImageInfo(img))
		}
	}

	session := &Session{
		ID:         generateID(),
		Directory:  dirPath,
		Filter:     filter,
		TargetKeep: targetKeep,
		Status:     StatusActive,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		images:     queue,
		queue:      queue,
		CurrentIdx: 0,
		undoStack:  make([]UndoEntry, 0),
	}

	m.mu.Lock()
	m.sessions[session.ID] = session
	m.mu.Unlock()

	return session, nil
}

func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, exists := m.sessions[id]
	return session, exists
}

func (m *Manager) MarkImage(sessionID string, imageID string, action Action) (*ImageInfo, *Stats, error) {
	session, exists := m.Get(sessionID)
	if !exists {
		return nil, nil, fmt.Errorf("session not found")
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	if session.Status != StatusActive {
		return nil, nil, fmt.Errorf("session is not active")
	}

	if session.CurrentIdx >= len(session.queue) {
		return nil, nil, fmt.Errorf("no more images")
	}

	currentImage := session.queue[session.CurrentIdx]
	if currentImage.ID() != imageID {
		found := false
		for i, img := range session.queue {
			if img.ID() == imageID {
				currentImage = img
				session.CurrentIdx = i
				found = true
				break
			}
		}
		if !found {
			return nil, nil, fmt.Errorf("image ID mismatch")
		}
	}

	session.undoStack = append(session.undoStack, UndoEntry{
		ImageID: imageID,
		Action:  currentImage.Action(),
	})

	currentImage.SetAction(action)
	session.UpdatedAt = time.Now()

	session.CurrentIdx++

	if session.CurrentIdx >= len(session.queue) {
		if session.Stats().Reviewed > 0 || session.Stats().Kept > 0 {
			var newQueue []*ImageInfo
			for _, img := range session.queue {
				action := img.Action()
				if action == ActionPending || action == ActionKeep {
					newQueue = append(newQueue, img)
				}
			}
			if len(newQueue) > 0 {
				session.queue = newQueue
				session.CurrentIdx = 0
				session.undoStack = make([]UndoEntry, 0)
			} else {
				session.Status = StatusCompleted
			}
		} else {
			session.Status = StatusCompleted
		}
	}

	stats := session.Stats()
	return currentImage, &stats, nil
}

func (m *Manager) Undo(sessionID string) (*ImageInfo, *Stats, error) {
	session, exists := m.Get(sessionID)
	if !exists {
		return nil, nil, fmt.Errorf("session not found")
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	if len(session.undoStack) == 0 {
		return nil, nil, fmt.Errorf("nothing to undo")
	}

	lastEntry := session.undoStack[len(session.undoStack)-1]
	session.undoStack = session.undoStack[:len(session.undoStack)-1]

	for _, img := range session.images {
		if img.ID() == lastEntry.ImageID {
			img.SetAction(lastEntry.Action)

			session.CurrentIdx--
			session.Status = StatusActive
			session.UpdatedAt = time.Now()

			stats := session.Stats()
			return img, &stats, nil
		}
	}

	return nil, nil, fmt.Errorf("image not found in undo stack")
}

func (m *Manager) CurrentImage(sessionID string) (*ImageInfo, error) {
	session, exists := m.Get(sessionID)
	if !exists {
		return nil, fmt.Errorf("session not found")
	}

	session.mu.RLock()
	defer session.mu.RUnlock()

	return session.CurrentImage(), nil
}

func (m *Manager) Commit(sessionID string, writeActions *WriteActions) (int, []error) {
	session, exists := m.Get(sessionID)
	if !exists {
		return 0, []error{fmt.Errorf("session not found")}
	}

	session.mu.Lock()
	session.Status = StatusCommitting
	session.mu.Unlock()

	var errors []error
	success := 0

	for _, img := range session.images {
		action := img.Action()

		var rating int
		switch action {
		case ActionKeep:
			rating = writeActions.KeepRating
		case ActionPending:
			rating = writeActions.PendingRating
		case ActionReject:
			rating = writeActions.RejectRating
		}
		if rating == 0 && !img.XMPExists() {
			continue
		}

		xmpData := &xmp.XMPData{
			Rating:    rating,
			Action:    string(action),
			SessionID: session.ID,
			Timestamp: time.Now(),
		}

		if err := xmp.Write(img.Path(), xmpData); err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", img.Filename(), err))
			continue
		}
		success++
	}

	session.mu.Lock()
	session.Status = StatusCompleted
	session.mu.Unlock()

	return success, errors
}

func convertImageInfo(img *scanner.ImageInfo) *ImageInfo {
	return &ImageInfo{
		id:            img.Path,
		filename:      img.Filename,
		path:          img.Path,
		size:          img.Size,
		currentRating: img.CurrentRating,
		xmpExists:     img.XMPExists,
		action:        ActionPending,
	}
}

func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func contains(slice []int, item int) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
