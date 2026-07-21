package sqlite

import (
	"context"
	"testing"
	"time"

	"main/internal/domain/notification"
	"main/internal/scalar"
	"main/internal/shared"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotificationRepository(t *testing.T) {
	// 使用内存数据库，避免文件系统依赖
	repo, cleanup, err := NewNotificationRepository(":memory:", notification.NewFilterBuilder())
	require.NoError(t, err)
	defer cleanup()

	ctx := context.Background()

	testURI := scalar.MustParseURI("open-dir://some/path?arg=1")

	// 1. 测试 Save (Create)
	id1 := scalar.ToID("notif:11111111-1111-1111-1111-111111111111")
	n1, err := notification.FromRepository(
		id1,
		"11111111-1111-1111-1111-111111111111",
		"hook:test",
		"Test Title 1",
		"Test Body 1",
		shared.NotificationPriorityNormal,
		time.Time{},
		time.Time{},
		time.Now().Add(24*365*10*time.Hour), // notAfter: 10 年后才过期
		time.Now().Add(-1*time.Hour),         // notBefore: 1 小时前已可见
		time.Now().Add(-10*time.Minute),
		time.Now().Add(-10*time.Minute),
		testURI,
	)
	require.NoError(t, err)

	didCreate, err := repo.Save(ctx, n1)
	require.NoError(t, err)
	assert.True(t, didCreate)

	// 2. 测试 Get & GetByTag
	got, err := repo.Get(ctx, id1.String())
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", got.Tag())
	assert.Equal(t, "hook:test", got.Channel())
	assert.Equal(t, "Test Title 1", got.Title())
	assert.Equal(t, testURI.String(), got.DetailsURL().String())

	gotByTag, err := repo.GetByTag(ctx, "11111111-1111-1111-1111-111111111111")
	require.NoError(t, err)
	require.NotNil(t, gotByTag)
	assert.Equal(t, id1, gotByTag.ID())
	assert.Equal(t, testURI.String(), gotByTag.DetailsURL().String())

	// 3. 测试 Save (Update - 同 tag)
	n1Updated, err := notification.FromRepository(
		scalar.ToID("notif:different-id-will-be-ignored-on-sqlite-upsert"),
		"11111111-1111-1111-1111-111111111111",
		"hook:test",
		"Updated Title",
		"Updated Body",
		shared.NotificationPriorityHigh,
		time.Time{},
		time.Time{},
		time.Now().Add(24*365*10*time.Hour),
		time.Now().Add(-1*time.Hour),
		time.Now(),
		time.Now(),
		testURI,
	)
	require.NoError(t, err)

	didCreate, err = repo.Save(ctx, n1Updated)
	require.NoError(t, err)
	assert.False(t, didCreate)

	// 验证 ID 依然为 id1 且内容已更新
	got, err = repo.GetByTag(ctx, "11111111-1111-1111-1111-111111111111")
	require.NoError(t, err)
	assert.Equal(t, id1, got.ID())
	assert.Equal(t, "Updated Title", got.Title())
	assert.Equal(t, shared.NotificationPriorityHigh, got.Priority())
	assert.Equal(t, testURI.String(), got.DetailsURL().String())

	// 4. 测试 Find (List)
	n2, err := notification.FromRepository(
		scalar.ToID("notif:22222222-2222-2222-2222-222222222222"),
		"22222222-2222-2222-2222-222222222222",
		"hook:another",
		"Test Title 2",
		"Test Body 2",
		shared.NotificationPriorityLow,
		time.Time{},
		time.Time{},
		time.Now().Add(24*365*10*time.Hour),
		time.Now().Add(-2*time.Hour),        // notBefore: 2 小时前
		time.Now(),
		time.Now(),
		scalar.URI{},
	)
	require.NoError(t, err)
	_, err = repo.Save(ctx, n2)
	require.NoError(t, err)

	// 4a. 无 Filter 查询
	var list []*notification.Notification
	for item, err := range repo.Find(ctx) {
		require.NoError(t, err)
		list = append(list, item)
	}
	assert.Len(t, list, 2)

	// 4b. 过滤特定 channel（数组形式匹配任意一个）
	var filtered []*notification.Notification
	for item, err := range repo.Find(ctx, notification.FindWithFilter(shared.NotificationFilters{Channel: []string{"hook:test"}})) {
		require.NoError(t, err)
		filtered = append(filtered, item)
	}
	assert.Len(t, filtered, 1)
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", filtered[0].Tag())

	// 5. 测试 Channels 聚合（按 latest_not_before DESC 排序）
	var channels []*notification.ChannelStats
	for ch, err := range repo.Channels(ctx) {
		require.NoError(t, err)
		channels = append(channels, ch)
	}
	assert.Len(t, channels, 2)

	// 确认排序和统计：hook:test (notBefore -1h) 应该在 hook:another (notBefore -2h) 之前
	assert.Equal(t, "hook:test", channels[0].Channel)
	assert.Equal(t, 1, channels[0].UnreadCount)
	assert.Equal(t, "notif:11111111-1111-1111-1111-111111111111", channels[0].LatestNotificationID.String())

	assert.Equal(t, "hook:another", channels[1].Channel)
	assert.Equal(t, 1, channels[1].UnreadCount)
	assert.Equal(t, "notif:22222222-2222-2222-2222-222222222222", channels[1].LatestNotificationID.String())

	// 6. 测试 Channels 被动增量刷新（pending 状态变为 visible 时）
	n3, err := notification.FromRepository(
		scalar.ToID("notif:33333333-3333-3333-3333-333333333333"),
		"33333333-3333-3333-3333-333333333333",
		"hook:pending",
		"Future Title",
		"Future Body",
		shared.NotificationPriorityNormal,
		time.Time{},
		time.Time{},
		time.Now().Add(24*time.Hour),
		time.Now().Add(10*time.Millisecond), // 10ms 后可见
		time.Now(),
		time.Now(),
		scalar.URI{},
	)
	require.NoError(t, err)
	_, err = repo.Save(ctx, n3)
	require.NoError(t, err)

	// Save 时未到 notBefore，LatestNotificationID 为空
	channels = nil
	for ch, err := range repo.Channels(ctx) {
		require.NoError(t, err)
		if ch.Channel == "hook:pending" {
			channels = append(channels, ch)
		}
	}
	require.Len(t, channels, 1)
	assert.Equal(t, "", channels[0].LatestNotificationID.String())

	// 等待 15ms 使 notBefore 过期，再次调用 Channels 触发被动增量刷新
	time.Sleep(15 * time.Millisecond)

	channels = nil
	for ch, err := range repo.Channels(ctx) {
		require.NoError(t, err)
		if ch.Channel == "hook:pending" {
			channels = append(channels, ch)
		}
	}
	require.Len(t, channels, 1)
	assert.Equal(t, "notif:33333333-3333-3333-3333-333333333333", channels[0].LatestNotificationID.String())
}