package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
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

	testURI := scalar.MustParseURI("open-dir://some/path?arg=1")

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
		testURI,
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
	assert.Equal(t, testURI.String(), got.DetailURL().String())

	gotByTag, err := repo.GetByTag(ctx, "tag-1")
	require.NoError(t, err)
	require.NotNil(t, gotByTag)
	assert.Equal(t, id1, gotByTag.ID())
	assert.Equal(t, testURI.String(), gotByTag.DetailURL().String())

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
		testURI,
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
	assert.Equal(t, testURI.String(), got.DetailURL().String())

	// 4. 测试 Find (List)
	n2 := notification.FromRepository(
		scalar.ToID("notif:2"),
		"tag-2",
		"hook:another",
		"Test Title 2",
		"Test Body 2",
		shared.NotificationPriorityLow,
		time.Time{},
		time.Time{},
		time.Time{},
		time.Time{},
		time.Now(),
		time.Now(),
		scalar.URI{},
	)
	_, err = repo.Save(ctx, n2)
	require.NoError(t, err)

	// 4a. 无 Filter 查询
	var list []*notification.Notification
	for item, err := range repo.Find(ctx) {
		require.NoError(t, err)
		list = append(list, item)
	}
	assert.Len(t, list, 2)

	// 4b. 过滤特定 channel
	var filtered []*notification.Notification
	ch := "hook:test"
	for item, err := range repo.Find(ctx, notification.FindWithFilter(shared.NotificationFilters{Channel: &ch})) {
		require.NoError(t, err)
		filtered = append(filtered, item)
	}
	assert.Len(t, filtered, 1)
	assert.Equal(t, "tag-1", filtered[0].Tag())

	// 5. 测试 Channels 聚合
	var channels []*notification.ChannelStats
	for ch, err := range repo.Channels(ctx) {
		require.NoError(t, err)
		channels = append(channels, ch)
	}
	assert.Len(t, channels, 2)

	// 确认排序和统计
	assert.Equal(t, "hook:another", channels[0].Channel)
	assert.Equal(t, 1, channels[0].UnreadCount)
	assert.Equal(t, "notif:2", channels[0].LatestNotification.ID().String())

	assert.Equal(t, "hook:test", channels[1].Channel)
	assert.Equal(t, 1, channels[1].UnreadCount)
	assert.Equal(t, "notif:1", channels[1].LatestNotification.ID().String())

	// 6. 测试物理删除 (IsDeleted = true)
	got.MarkDeleted()
	didCreate, err = repo.Save(ctx, got)
	require.NoError(t, err)
	assert.False(t, didCreate)

	// 验证已删除
	deletedGot, err := repo.Get(ctx, got.ID().String())
	require.NoError(t, err)
	assert.Nil(t, deletedGot)
}

func TestNotificationRepository_Version(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "image-funnel-sqlite-version-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// 1. 初始化，校验 user_version 是否为 2
	repo, err := NewNotificationRepository(tempDir)
	require.NoError(t, err)

	var dbVersion int
	err = repo.db.QueryRow("PRAGMA user_version;").Scan(&dbVersion)
	require.NoError(t, err)
	assert.Equal(t, 2, dbVersion)
	repo.Close()

	// 2. 打开 db 并手动升级为高版本 3
	dbPath := filepath.Join(tempDir, "notifications.db")
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = db.Exec("PRAGMA user_version = 3;")
	require.NoError(t, err)
	db.Close()

	// 3. 用老代码重新连接高版本数据库，必须报错拒绝操作
	_, err = NewNotificationRepository(tempDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database schema version 3 is newer than expected version 2")
}
