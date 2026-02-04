package session

import (
	"main/internal/shared"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStats_AfterMarkingImages(t *testing.T) {
	session := setupTestSession(t, 10, 5)

	// 标记前3张为 KEEP
	for i := 0; i < 3; i++ {
		err := session.MarkImage(session.queue[i].ID(), shared.ImageActionKeep)
		require.NoError(t, err)
	}

	// 标记中间3张为 SHELVE
	for i := 3; i < 6; i++ {
		err := session.MarkImage(session.queue[i].ID(), shared.ImageActionShelve)
		require.NoError(t, err)
	}

	// 标记后3张为 REJECT
	for i := 6; i < 9; i++ {
		err := session.MarkImage(session.queue[i].ID(), shared.ImageActionReject)
		require.NoError(t, err)
	}

	stats := session.Stats()

	assert.Equal(t, 10, stats.Total, "Total should be 10")
	assert.Equal(t, 9, session.CurrentIndex(), "Processed should be 9")
	assert.Equal(t, 3, stats.Kept, "Kept should be 3")
	assert.Equal(t, 3, stats.Shelved, "Shelved should be 3")
	assert.Equal(t, 3, stats.Rejected, "Rejected should be 3")
	assert.Equal(t, 1, stats.Remaining, "Remaining should be 1")
}
