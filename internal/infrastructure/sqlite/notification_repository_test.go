package sqlite

import (
	"context"
	"os"
	"testing"
	"time"

	"main/internal/domain/notification"
	"main/internal/scalar"
	"main/internal/shared"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotificationRepository(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "image-funnel-sqlite-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	repo, err := NewNotificationRepository(tempDir)
	require.NoError(t, err)
	defer repo.Close()

	ctx := context.Background()

	// 1. 测试 Save (Create)
	id1 := scalar.ToID("notif:1")
	n1 := notification.FromRepository(
		id1,
		"tag-1",
		"hook:test",
		"Test Title 1",
		"Test Body 1",
		shared.NotificationPriorityNormal,
		time.Time{},
		time.Time{},
		time.Time{},
		time.Time{},
		time.Now().Add(-10*time.Minute),
		time.Now().Add(-10*time.Minute),
	)

	didCreate, err := repo.Save(ctx, n1)
	require.NoError(t, err)
	assert.True(t, didCreate)

	// 2. 测试 Get & GetByTag
	got, err := repo.Get(ctx, id1.String())
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "tag-1", got.Tag())
	assert.Equal(t, "hook:test", got.Channel())
	assert.Equal(t, "Test Title 1", got.Title())

	gotByTag, err := repo.GetByTag(ctx, "tag-1")
	require.NoError(t, err)
	require.NotNil(t, gotByTag)
	assert.Equal(t, id1, gotByTag.ID())

	// 3. 测试 Save (Update - 同 tag)
	n1Updated := notification.FromRepository(
		scalar.ToID("notif:different-id-will-be-ignored-on-sqlite-upsert"),
		"tag-1",
		"hook:test",
		"Updated Title",
		"Updated Body",
		shared.NotificationPriorityHigh,
		time.Time{},
		time.Time{},
		time.Time{},
		time.Time{},
		time.Now(),
		time.Now(),
	)

	didCreate, err = repo.Save(ctx, n1Updated)
	require.NoError(t, err)
	assert.False(t, didCreate)

	// 验证 ID 依然为 id1 且内容已更新
	got, err = repo.GetByTag(ctx, "tag-1")
	require.NoError(t, err)
	assert.Equal(t, id1, got.ID())
	assert.Equal(t, "Updated Title", got.Title())
	assert.Equal(t, shared.NotificationPriorityHigh, got.Priority())

	// 4. 测试 Find & Channels
	// 插入另一条通知
	id2 := scalar.ToID("notif:2")
	n2 := notification.FromRepository(
		id2,
		"tag-2",
		"hook:another",
		"Test Title 2",
		"Test Body 2",
		shared.NotificationPriorityLow,
		time.Time{},
		time.Time{},
		time.Time{},
		time.Time{},
		time.Now().Add(-5*time.Minute),
		time.Now().Add(-5*time.Minute),
	)
	didCreate, err = repo.Save(ctx, n2)
	require.NoError(t, err)
	assert.True(t, didCreate)

	// Find - 全量
	var found []*notification.Notification
	for item, err := range repo.Find(ctx) {
		require.NoError(t, err)
		found = append(found, item)
	}
	assert.Len(t, found, 2)

	// Find - 带 channel 过滤
	chFilter := "hook:another"
	var foundFiltered []*notification.Notification
	for item, err := range repo.Find(ctx, notification.FindWithFilter(shared.NotificationFilters{
		Channel: &chFilter,
	})) {
		require.NoError(t, err)
		foundFiltered = append(foundFiltered, item)
	}
	require.Len(t, foundFiltered, 1)
	assert.Equal(t, "tag-2", foundFiltered[0].Tag())

	// Channels stats
	var channels []*notification.ChannelStats
	for ch, err := range repo.Channels(ctx) {
		require.NoError(t, err)
		channels = append(channels, ch)
	}
	assert.Len(t, channels, 2)

	var hookTestStats *notification.ChannelStats
	for _, c := range channels {
		if c.Channel == "hook:test" {
			hookTestStats = c
		}
	}
	require.NotNil(t, hookTestStats)
	assert.Equal(t, 1, hookTestStats.UnreadCount)
	assert.Equal(t, "Updated Title", hookTestStats.LatestNotification.Title())

	// 5. 测试物理删除 (MarkDeleted -> Save)
	got.MarkDeleted()
	_, err = repo.Save(ctx, got)
	require.NoError(t, err)

	// 验证已删除
	deletedGot, err := repo.Get(ctx, got.ID().String())
	require.NoError(t, err)
	assert.Nil(t, deletedGot)
}
