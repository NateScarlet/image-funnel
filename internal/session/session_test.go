package session

import (
	"fmt"
	"testing"

	"main/internal/scanner"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarkImage_AllKeep_ShouldStartNextRound(t *testing.T) {
	manager := NewManager()

	testFilter := &ImageFilters{
		Rating: []int{0},
	}

	images := make([]*scanner.ImageInfo, 10)
	for i := 0; i < 10; i++ {
		images[i] = &scanner.ImageInfo{
			ID:            fmt.Sprintf("img-%d", i),
			Filename:      "test.jpg",
			Path:          fmt.Sprintf("/test/test-%d.jpg", i),
			Size:          1000,
			CurrentRating: 0,
			XMPExists:     false,
		}
	}

	session := &Session{
		ID:         "test-session",
		Directory:  "/test",
		Filter:     testFilter,
		TargetKeep: 5,
		Status:     StatusActive,
		images:     convertImages(images),
		queue:      convertImages(images),
		CurrentIdx: 0,
		undoStack:  make([]UndoEntry, 0),
	}

	manager.sessions[session.ID] = session

	initialStats := session.Stats()
	assert.Equal(t, 0, initialStats.Processed, "Initial processed should be 0")
	assert.Equal(t, 10, initialStats.Total, "Initial total should be 10")
	assert.Equal(t, 10, initialStats.Remaining, "Initial remaining should be 10")

	for i := 0; i < 10; i++ {
		_, _, err := manager.MarkImage(session.ID, session.queue[i].ID(), ActionKeep)
		require.NoError(t, err, "MarkImage should not return error")
	}

	assert.Equal(t, StatusActive, session.Status, "Session status should be ACTIVE")
	assert.Equal(t, 10, len(session.queue), "Queue length should be 10")
	assert.Equal(t, 0, session.CurrentIdx, "CurrentIdx should be 0")

	finalStats := session.Stats()
	assert.Equal(t, 0, finalStats.Processed, "Final processed should be 0")
	assert.Equal(t, 10, finalStats.Total, "Final total should be 10")
	assert.Equal(t, 10, finalStats.Remaining, "Final remaining should be 10")
}

func TestMarkImage_AllReject_ShouldComplete(t *testing.T) {
	manager := NewManager()

	testFilter := &ImageFilters{
		Rating: []int{0},
	}

	images := make([]*scanner.ImageInfo, 10)
	for i := 0; i < 10; i++ {
		images[i] = &scanner.ImageInfo{
			ID:            fmt.Sprintf("img-%d", i),
			Filename:      "test.jpg",
			Path:          fmt.Sprintf("/test/test-%d.jpg", i),
			Size:          1000,
			CurrentRating: 0,
			XMPExists:     false,
		}
	}

	session := &Session{
		ID:         "test-session",
		Directory:  "/test",
		Filter:     testFilter,
		TargetKeep: 5,
		Status:     StatusActive,
		images:     convertImages(images),
		queue:      convertImages(images),
		CurrentIdx: 0,
		undoStack:  make([]UndoEntry, 0),
	}

	manager.sessions[session.ID] = session

	initialStats := session.Stats()
	assert.Equal(t, 0, initialStats.Processed, "Initial processed should be 0")

	for i := 0; i < 10; i++ {
		_, _, err := manager.MarkImage(session.ID, session.queue[i].ID(), ActionReject)
		require.NoError(t, err, "MarkImage should not return error")
	}

	assert.Equal(t, StatusCompleted, session.Status, "Session status should be COMPLETED")

	finalStats := session.Stats()
	assert.Equal(t, 10, finalStats.Processed, "Final processed should be 10")
	assert.Equal(t, 10, finalStats.Rejected, "Final rejected should be 10")
}

func TestMarkImage_WithReview_ShouldStartNextRound(t *testing.T) {
	manager := NewManager()

	testFilter := &ImageFilters{
		Rating: []int{0},
	}

	images := make([]*scanner.ImageInfo, 10)
	for i := 0; i < 10; i++ {
		images[i] = &scanner.ImageInfo{
			ID:            fmt.Sprintf("img-%d", i),
			Filename:      "test.jpg",
			Path:          fmt.Sprintf("/test/test-%d.jpg", i),
			Size:          1000,
			CurrentRating: 0,
			XMPExists:     false,
		}
	}

	session := &Session{
		ID:         "test-session",
		Directory:  "/test",
		Filter:     testFilter,
		TargetKeep: 5,
		Status:     StatusActive,
		images:     convertImages(images),
		queue:      convertImages(images),
		CurrentIdx: 0,
		undoStack:  make([]UndoEntry, 0),
	}

	manager.sessions[session.ID] = session

	initialStats := session.Stats()
	assert.Equal(t, 0, initialStats.Processed, "Initial processed should be 0")
	assert.Equal(t, 10, initialStats.Total, "Initial total should be 10")
	assert.Equal(t, 10, initialStats.Remaining, "Initial remaining should be 10")

	for i := 0; i < 10; i++ {
		action := ActionKeep
		if i%3 == 0 {
			action = ActionPending
		} else if i%3 == 1 {
			action = ActionReject
		}
		t.Logf("Marking image %s with action %s", session.queue[i].ID(), action)
		_, _, err := manager.MarkImage(session.ID, session.queue[i].ID(), action)
		require.NoError(t, err, "MarkImage should not return error")
		stats := session.Stats()
		t.Logf("After marking: Kept=%d, Reviewed=%d, Rejected=%d, Status=%s, CurrentIdx=%d, QueueLen=%d",
			stats.Kept, stats.Reviewed, stats.Rejected, session.Status, session.CurrentIdx, len(session.queue))
	}

	assert.Equal(t, StatusActive, session.Status, "Session status should be ACTIVE")

	newRoundStats := session.Stats()
	assert.Equal(t, 0, newRoundStats.Processed, "New round processed should be 0")
	assert.Equal(t, 7, newRoundStats.Total, "New round total should be 7")
	assert.Equal(t, 7, newRoundStats.Remaining, "New round remaining should be 7")

	for i, img := range session.queue {
		t.Logf("Queue[%d]: ID=%s, Action=%s", i, img.ID(), img.Action())
	}

	expectedQueueLength := 7
	assert.Equal(t, expectedQueueLength, len(session.queue), "Queue length should be %d", expectedQueueLength)
	assert.Equal(t, 0, session.CurrentIdx, "CurrentIdx should be 0")
}

func TestMarkImage_KeepAndReview_ShouldStartNextRoundWithBoth(t *testing.T) {
	manager := NewManager()

	testFilter := &ImageFilters{
		Rating: []int{0},
	}

	images := make([]*scanner.ImageInfo, 10)
	for i := 0; i < 10; i++ {
		images[i] = &scanner.ImageInfo{
			ID:            fmt.Sprintf("img-%d", i),
			Filename:      "test.jpg",
			Path:          fmt.Sprintf("/test/test-%d.jpg", i),
			Size:          1000,
			CurrentRating: 0,
			XMPExists:     false,
		}
	}

	session := &Session{
		ID:         "test-session",
		Directory:  "/test",
		Filter:     testFilter,
		TargetKeep: 5,
		Status:     StatusActive,
		images:     convertImages(images),
		queue:      convertImages(images),
		CurrentIdx: 0,
		undoStack:  make([]UndoEntry, 0),
	}

	manager.sessions[session.ID] = session

	for i := 0; i < 10; i++ {
		action := ActionKeep
		if i%2 == 0 {
			action = ActionPending
		}
		_, _, err := manager.MarkImage(session.ID, session.queue[i].ID(), action)
		require.NoError(t, err, "MarkImage should not return error")
	}

	assert.Equal(t, StatusActive, session.Status, "Session status should be ACTIVE")

	expectedQueueLength := 10
	assert.Equal(t, expectedQueueLength, len(session.queue), "Queue length should be %d", expectedQueueLength)
	assert.Equal(t, 0, session.CurrentIdx, "CurrentIdx should be 0")
}

func convertImages(images []*scanner.ImageInfo) []*ImageInfo {
	result := make([]*ImageInfo, len(images))
	for i, img := range images {
		result[i] = convertImageInfo(img)
	}
	return result
}

func TestCanCommit_InitialState_ShouldReturnFalse(t *testing.T) {
	testFilter := &ImageFilters{
		Rating: []int{0},
	}

	images := make([]*scanner.ImageInfo, 10)
	for i := 0; i < 10; i++ {
		images[i] = &scanner.ImageInfo{
			ID:            fmt.Sprintf("img-%d", i),
			Filename:      "test.jpg",
			Path:          fmt.Sprintf("/test/test-%d.jpg", i),
			Size:          1000,
			CurrentRating: 0,
			XMPExists:     false,
		}
	}

	session := &Session{
		ID:         "test-session",
		Directory:  "/test",
		Filter:     testFilter,
		TargetKeep: 5,
		Status:     StatusActive,
		images:     convertImages(images),
		queue:      convertImages(images),
		CurrentIdx: 0,
		undoStack:  make([]UndoEntry, 0),
	}

	assert.False(t, session.CanCommit(), "CanCommit should return false for initial state")
}

func TestCanCommit_AfterMarkingImages_ShouldReturnTrue(t *testing.T) {
	testFilter := &ImageFilters{
		Rating: []int{0},
	}

	images := make([]*scanner.ImageInfo, 10)
	for i := 0; i < 10; i++ {
		images[i] = &scanner.ImageInfo{
			ID:            fmt.Sprintf("img-%d", i),
			Filename:      "test.jpg",
			Path:          fmt.Sprintf("/test/test-%d.jpg", i),
			Size:          1000,
			CurrentRating: 0,
			XMPExists:     false,
		}
	}

	session := &Session{
		ID:         "test-session",
		Directory:  "/test",
		Filter:     testFilter,
		TargetKeep: 5,
		Status:     StatusActive,
		images:     convertImages(images),
		queue:      convertImages(images),
		CurrentIdx: 0,
		undoStack:  make([]UndoEntry, 0),
	}

	for i := 0; i < 3; i++ {
		session.queue[i].SetAction(ActionKeep)
		session.CurrentIdx++
	}

	assert.True(t, session.CanCommit(), "CanCommit should return true after marking images")
}

func TestCanCommit_CommittingStatus_ShouldReturnFalse(t *testing.T) {
	testFilter := &ImageFilters{
		Rating: []int{0},
	}

	images := make([]*scanner.ImageInfo, 10)
	for i := 0; i < 10; i++ {
		images[i] = &scanner.ImageInfo{
			ID:            fmt.Sprintf("img-%d", i),
			Filename:      "test.jpg",
			Path:          fmt.Sprintf("/test/test-%d.jpg", i),
			Size:          1000,
			CurrentRating: 0,
			XMPExists:     false,
		}
	}

	session := &Session{
		ID:         "test-session",
		Directory:  "/test",
		Filter:     testFilter,
		TargetKeep: 5,
		Status:     StatusCommitting,
		images:     convertImages(images),
		queue:      convertImages(images),
		CurrentIdx: 3,
		undoStack:  make([]UndoEntry, 0),
	}

	assert.False(t, session.CanCommit(), "CanCommit should return false for COMMITTING status")
}

func TestCanCommit_ErrorStatus_ShouldReturnFalse(t *testing.T) {
	testFilter := &ImageFilters{
		Rating: []int{0},
	}

	images := make([]*scanner.ImageInfo, 10)
	for i := 0; i < 10; i++ {
		images[i] = &scanner.ImageInfo{
			ID:            fmt.Sprintf("img-%d", i),
			Filename:      "test.jpg",
			Path:          fmt.Sprintf("/test/test-%d.jpg", i),
			Size:          1000,
			CurrentRating: 0,
			XMPExists:     false,
		}
	}

	session := &Session{
		ID:         "test-session",
		Directory:  "/test",
		Filter:     testFilter,
		TargetKeep: 5,
		Status:     StatusError,
		images:     convertImages(images),
		queue:      convertImages(images),
		CurrentIdx: 3,
		undoStack:  make([]UndoEntry, 0),
	}

	assert.False(t, session.CanCommit(), "CanCommit should return false for ERROR status")
}

func TestCanUndo_InitialState_ShouldReturnFalse(t *testing.T) {
	testFilter := &ImageFilters{
		Rating: []int{0},
	}

	images := make([]*scanner.ImageInfo, 10)
	for i := 0; i < 10; i++ {
		images[i] = &scanner.ImageInfo{
			ID:            fmt.Sprintf("img-%d", i),
			Filename:      "test.jpg",
			Path:          fmt.Sprintf("/test/test-%d.jpg", i),
			Size:          1000,
			CurrentRating: 0,
			XMPExists:     false,
		}
	}

	session := &Session{
		ID:         "test-session",
		Directory:  "/test",
		Filter:     testFilter,
		TargetKeep: 5,
		Status:     StatusActive,
		images:     convertImages(images),
		queue:      convertImages(images),
		CurrentIdx: 0,
		undoStack:  make([]UndoEntry, 0),
	}

	assert.False(t, session.CanUndo(), "CanUndo should return false for initial state")
}

func TestCanUndo_AfterMarkingImages_ShouldReturnTrue(t *testing.T) {
	testFilter := &ImageFilters{
		Rating: []int{0},
	}

	images := make([]*scanner.ImageInfo, 10)
	for i := 0; i < 10; i++ {
		images[i] = &scanner.ImageInfo{
			ID:            fmt.Sprintf("img-%d", i),
			Filename:      "test.jpg",
			Path:          fmt.Sprintf("/test/test-%d.jpg", i),
			Size:          1000,
			CurrentRating: 0,
			XMPExists:     false,
		}
	}

	session := &Session{
		ID:         "test-session",
		Directory:  "/test",
		Filter:     testFilter,
		TargetKeep: 5,
		Status:     StatusActive,
		images:     convertImages(images),
		queue:      convertImages(images),
		CurrentIdx: 0,
		undoStack:  make([]UndoEntry, 0),
	}

	session.undoStack = append(session.undoStack, UndoEntry{
		ImageID: "img-0",
		Action:  ActionPending,
	})

	assert.True(t, session.CanUndo(), "CanUndo should return true after marking images")
}

func TestCanUndo_AfterUndoAll_ShouldReturnFalse(t *testing.T) {
	testFilter := &ImageFilters{
		Rating: []int{0},
	}

	images := make([]*scanner.ImageInfo, 10)
	for i := 0; i < 10; i++ {
		images[i] = &scanner.ImageInfo{
			ID:            fmt.Sprintf("img-%d", i),
			Filename:      "test.jpg",
			Path:          fmt.Sprintf("/test/test-%d.jpg", i),
			Size:          1000,
			CurrentRating: 0,
			XMPExists:     false,
		}
	}

	session := &Session{
		ID:         "test-session",
		Directory:  "/test",
		Filter:     testFilter,
		TargetKeep: 5,
		Status:     StatusActive,
		images:     convertImages(images),
		queue:      convertImages(images),
		CurrentIdx: 0,
		undoStack:  make([]UndoEntry, 0),
	}

	assert.False(t, session.CanUndo(), "CanUndo should return false when undoStack is empty")
}

func TestCanCommit_FirstRoundWithRejects_SecondRoundStart_ShouldBeAbleToCommit(t *testing.T) {
	manager := NewManager()

	testFilter := &ImageFilters{
		Rating: []int{0},
	}

	images := make([]*scanner.ImageInfo, 10)
	for i := 0; i < 10; i++ {
		images[i] = &scanner.ImageInfo{
			ID:            fmt.Sprintf("img-%d", i),
			Filename:      "test.jpg",
			Path:          fmt.Sprintf("/test/test-%d.jpg", i),
			Size:          1000,
			CurrentRating: 0,
			XMPExists:     false,
		}
	}

	session := &Session{
		ID:         "test-session",
		Directory:  "/test",
		Filter:     testFilter,
		TargetKeep: 5,
		Status:     StatusActive,
		images:     convertImages(images),
		queue:      convertImages(images),
		CurrentIdx: 0,
		undoStack:  make([]UndoEntry, 0),
	}

	manager.sessions[session.ID] = session

	for i := 0; i < 10; i++ {
		action := ActionKeep
		if i%2 == 0 {
			action = ActionReject
		}
		_, _, err := manager.MarkImage(session.ID, session.queue[i].ID(), action)
		require.NoError(t, err, "MarkImage should not return error")
	}

	assert.Equal(t, StatusActive, session.Status, "Session status should be ACTIVE")
	assert.True(t, session.CanCommit(), "CanCommit should return true at start of second round (with rejected images)")

	stats := session.Stats()
	assert.Equal(t, 5, stats.Rejected, "Expected 5 rejected images")
}

func TestCanCommit_FirstRoundOnlyRejects_SecondRoundStart_ShouldBeAbleToCommit(t *testing.T) {
	manager := NewManager()

	testFilter := &ImageFilters{
		Rating: []int{0},
	}

	images := make([]*scanner.ImageInfo, 10)
	for i := 0; i < 10; i++ {
		images[i] = &scanner.ImageInfo{
			ID:            fmt.Sprintf("img-%d", i),
			Filename:      "test.jpg",
			Path:          fmt.Sprintf("/test/test-%d.jpg", i),
			Size:          1000,
			CurrentRating: 0,
			XMPExists:     false,
		}
	}

	session := &Session{
		ID:         "test-session",
		Directory:  "/test",
		Filter:     testFilter,
		TargetKeep: 5,
		Status:     StatusActive,
		images:     convertImages(images),
		queue:      convertImages(images),
		CurrentIdx: 0,
		undoStack:  make([]UndoEntry, 0),
	}

	manager.sessions[session.ID] = session

	for i := 0; i < 10; i++ {
		_, _, err := manager.MarkImage(session.ID, session.queue[i].ID(), ActionReject)
		require.NoError(t, err, "MarkImage should not return error")
	}

	assert.Equal(t, StatusCompleted, session.Status, "Session status should be COMPLETED when all images are rejected")
	assert.True(t, session.CanCommit(), "CanCommit should return true after completing with rejected images")
}

func TestCanCommit_FirstRoundSingleReject_ShouldBeAbleToCommit(t *testing.T) {
	manager := NewManager()

	testFilter := &ImageFilters{
		Rating: []int{0},
	}

	images := make([]*scanner.ImageInfo, 10)
	for i := 0; i < 10; i++ {
		images[i] = &scanner.ImageInfo{
			ID:            fmt.Sprintf("img-%d", i),
			Filename:      "test.jpg",
			Path:          fmt.Sprintf("/test/test-%d.jpg", i),
			Size:          1000,
			CurrentRating: 0,
			XMPExists:     false,
		}
	}

	session := &Session{
		ID:         "test-session",
		Directory:  "/test",
		Filter:     testFilter,
		TargetKeep: 5,
		Status:     StatusActive,
		images:     convertImages(images),
		queue:      convertImages(images),
		CurrentIdx: 0,
		undoStack:  make([]UndoEntry, 0),
	}

	manager.sessions[session.ID] = session

	_, _, err := manager.MarkImage(session.ID, session.queue[0].ID(), ActionReject)
	require.NoError(t, err, "MarkImage should not return error")

	assert.Equal(t, StatusActive, session.Status, "Session status should be ACTIVE")
	assert.True(t, session.CanCommit(), "CanCommit should return true after rejecting one image in first round")

	stats := session.Stats()
	assert.Equal(t, 1, stats.Rejected, "Expected 1 rejected image")
}

func TestMarkImage_KeptInFirstRound_ShouldKeepStatusInSecondRound(t *testing.T) {
	manager := NewManager()

	testFilter := &ImageFilters{
		Rating: []int{0},
	}

	images := make([]*scanner.ImageInfo, 10)
	for i := 0; i < 10; i++ {
		images[i] = &scanner.ImageInfo{
			ID:            fmt.Sprintf("img-%d", i),
			Filename:      "test.jpg",
			Path:          fmt.Sprintf("/test/test-%d.jpg", i),
			Size:          1000,
			CurrentRating: 0,
			XMPExists:     false,
		}
	}

	session := &Session{
		ID:         "test-session",
		Directory:  "/test",
		Filter:     testFilter,
		TargetKeep: 5,
		Status:     StatusActive,
		images:     convertImages(images),
		queue:      convertImages(images),
		CurrentIdx: 0,
		undoStack:  make([]UndoEntry, 0),
	}

	manager.sessions[session.ID] = session

	keptImageIDs := make(map[string]bool)

	for i := 0; i < 10; i++ {
		action := ActionKeep
		if i%3 == 0 {
			action = ActionPending
		} else if i%3 == 1 {
			action = ActionReject
		}
		imageID := session.queue[i].ID()
		if action == ActionKeep {
			keptImageIDs[imageID] = true
		}
		_, _, err := manager.MarkImage(session.ID, imageID, action)
		require.NoError(t, err, "MarkImage should not return error")
	}

	assert.Equal(t, StatusActive, session.Status, "Session status should be ACTIVE")

	expectedQueueLength := 7
	assert.Equal(t, expectedQueueLength, len(session.queue), "Queue length should be %d", expectedQueueLength)
	assert.Equal(t, 0, session.CurrentIdx, "CurrentIdx should be 0")

	for _, img := range session.queue {
		if keptImageIDs[img.ID()] {
			assert.Equal(t, ActionKeep, img.Action(), "Image %s was marked as KEEP in first round, but action is %s in second round", img.ID(), img.Action())
		}
	}
}
