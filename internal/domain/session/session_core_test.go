package session

import (
	"main/internal/scalar"
	"main/internal/shared"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSession_ShouldInitializeCorrectly(t *testing.T) {
	filter := &shared.ImageFilters{Rating: []int{0, 1, 2}}
	images := createTestImages(10)

	session := NewSession(scalar.ToID("test-id"), scalar.ToID("test-dir-id"), filter, 5, images)

	assert.NotEmpty(t, session.ID(), "Session ID should not be empty")
	assert.Equal(t, scalar.ToID("test-dir-id"), session.DirectoryID(), "DirectoryID should match")
	assert.Equal(t, filter, session.Filter(), "Filter should match")
	assert.Equal(t, 5, session.TargetKeep(), "TargetKeep should match")
	assert.False(t, session.Stats().IsCompleted(), "IsCompleted should be false initially")
	assert.Equal(t, 10, len(ImagesOf(session)), "Images count should match")
	assert.Equal(t, 10, len(session.queue), "Queue count should match")
	assert.Equal(t, 0, session.CurrentIndex(), "CurrentIndex should be 0")
	assert.False(t, session.CanUndo(), "CanUndo should be false initially")
}

func TestStats_InitialState(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	stats := session.Stats()

	assert.Equal(t, 10, stats.Total(), "Total should be 10")
	assert.Equal(t, 0, session.CurrentIndex(), "Processed should be 0")
	assert.Equal(t, 0, stats.Kept(), "Kept should be 0")
	assert.Equal(t, 0, stats.Shelved(), "Shelved should be 0")
	assert.Equal(t, 0, stats.Rejected(), "Rejected should be 0")
	assert.Equal(t, 10, stats.Remaining(), "Remaining should be 10")
}

func TestStats_Getters(t *testing.T) {
	stats := &Stats{
		total:     10,
		kept:      2,
		shelved:   2,
		rejected:  1,
		remaining: 5,
	}

	assert.Equal(t, 10, stats.Total(), "Total should match")
	assert.Equal(t, 2, stats.Kept(), "Kept should match")
	assert.Equal(t, 2, stats.Shelved(), "Shelved should match")
	assert.Equal(t, 1, stats.Rejected(), "Rejected should match")
	assert.Equal(t, 5, stats.Remaining(), "Remaining should match")
}

func TestWriteActions_Fields(t *testing.T) {
	actions := NewWriteActions(5, 3, 1)

	assert.Equal(t, 5, actions.keepRating, "keepRating should match")
	assert.Equal(t, 3, actions.shelveRating, "shelveRating should match")
	assert.Equal(t, 1, actions.rejectRating, "rejectRating should match")
}

func TestActions_ShouldOnlyReturnMarkedImages(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	// Mark only one image
	err := session.MarkImage(session.queue[0].ID(), shared.ImageActionKeep)
	require.NoError(t, err)

	count := 0
	for range session.Actions() {
		count++
	}

	assert.Equal(t, 1, count, "Actions should only return explicitly marked images")

	// Verify the specific action
	found := false
	for _, action := range session.Actions() {
		if action == shared.ImageActionKeep {
			found = true
		}
	}
	assert.True(t, found, "Should contain the marked action")
}
