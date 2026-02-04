package session

import (
	"main/internal/shared"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMarkImage_WithShelvedAndKept_ShouldCompleteIfKeptBelowTarget(t *testing.T) {
	// 目标保留 5 张
	session := setupTestSession(t, 10, 5)

	// 标记：2 张保留，8 张搁置 (Shelve)
	// 期望：因为 2 <= 5，且搁置视为不再处理，所以会话应该完成，而不是开启新一轮
	markImagesInSession(t, session, func(index int) shared.ImageAction {
		if index < 2 {
			return shared.ImageActionKeep
		}
		return shared.ImageActionShelve
	})

	stats := session.Stats()
	assert.Equal(t, 0, stats.Remaining, "All images should be processed")
	assert.Equal(t, 2, stats.Kept, "Should have 2 kept images")
	assert.Equal(t, 8, stats.Shelved, "Should have 8 shelved images")
	assert.True(t, stats.IsCompleted, "Session should be completed because shelved images are ignored")
}
