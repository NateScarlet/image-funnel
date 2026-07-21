package sqlite

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"main/internal/domain/notification"
	"main/internal/scalar"
	"main/internal/shared"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExplainQueryPlans 用于手动校验与打印所有核心 SQL 语句的 SQLite EXPLAIN QUERY PLAN 结果
func TestExplainQueryPlans(t *testing.T) {
	t.Skip("手动校验 EXPLAIN QUERY PLAN 时取消 Skip 运行")

	repo, cleanup, err := NewNotificationRepository("file::memory:?mode=memory&cache=shared", notification.NewFilterBuilder())
	require.NoError(t, err)
	defer cleanup()

	ctx := context.Background()

	queries := map[string]string{
		"Get":             `EXPLAIN QUERY PLAN SELECT id, tag, channel, title, body, priority, read_at, dismissed_at, not_after, not_before, created_at, updated_at, details_url FROM notifications WHERE id = ?`,
		"GetByTag":        `EXPLAIN QUERY PLAN SELECT id, tag, channel, title, body, priority, read_at, dismissed_at, not_after, not_before, created_at, updated_at, details_url FROM notifications WHERE tag = ?`,
		"SaveCheckTag":    `EXPLAIN QUERY PLAN SELECT COUNT(*) FROM notifications WHERE tag = ?`,
		"Find":            `EXPLAIN QUERY PLAN SELECT id, tag, channel, title, body, priority, read_at, dismissed_at, not_after, not_before, created_at, updated_at, details_url FROM notifications WHERE 1=1 AND channel IN ('hook:test') AND not_before <= 100 AND not_after > 100 ORDER BY created_at DESC, id DESC`,
		"Channels":        `EXPLAIN QUERY PLAN SELECT channel, unread_count, latest_notification_id, latest_not_before, expires_at FROM channel_summary ORDER BY CASE WHEN expires_at <= :now THEN 0 ELSE 1 END ASC, latest_not_before DESC, channel ASC`,
		"freshRows":       `EXPLAIN QUERY PLAN SELECT channel, unread_count, latest_notification_id, latest_not_before FROM channel_summary WHERE expires_at > :now AND latest_notification_id != '' ORDER BY latest_not_before DESC, channel ASC`,
		"refreshSummary":  `EXPLAIN QUERY PLAN SELECT channel, unread_count, latest_notification_id, latest_not_before FROM channel_summary WHERE channel = ?`,
		"refreshTxDelete": `EXPLAIN QUERY PLAN DELETE FROM channel_summary WHERE channel = :channel AND (SELECT COUNT(*) FROM notifications WHERE channel = :channel AND not_before <= :now AND not_after > :now) = 0 AND (SELECT COUNT(*) FROM notifications WHERE channel = :channel AND not_before > :now) = 0`,
		"refreshTxUpsert": `EXPLAIN QUERY PLAN WITH visible AS (SELECT id, not_before FROM notifications WHERE channel = :channel AND not_before <= :now AND not_after > :now ORDER BY not_before DESC, id DESC LIMIT 1) INSERT OR REPLACE INTO channel_summary (channel, unread_count, latest_notification_id, latest_not_before, expires_at) SELECT :channel, (SELECT COUNT(*) FROM notifications WHERE channel = :channel AND read_at = :zeroTimeMs AND not_before <= :now AND not_after > :now), COALESCE((SELECT id FROM visible), ''), COALESCE((SELECT not_before FROM visible), 0), COALESCE((SELECT MIN(t) FROM (SELECT not_before AS t FROM notifications WHERE channel = :channel AND not_before > :now UNION ALL SELECT not_after AS t FROM notifications WHERE channel = :channel AND not_before <= :now AND not_after > :now)), 0) WHERE (SELECT COUNT(*) FROM visible) > 0 OR (SELECT COUNT(*) FROM notifications WHERE channel = :channel AND not_before > :now) > 0`,
		"reclaimFind":     `EXPLAIN QUERY PLAN SELECT DISTINCT channel FROM notifications WHERE not_after <= :cutoff`,
		"reclaimDelete":   `EXPLAIN QUERY PLAN DELETE FROM notifications WHERE not_after <= :cutoff`,
	}
	for name, q := range queries {
		dummyArgs := []any{
			sql.Named("channel", "test"),
			sql.Named("now", int64(0)),
			sql.Named("zeroTimeMs", zeroTimeMs),
			sql.Named("cutoff", int64(0)),
			"test", "test", "test", 0, 0, 0, 0,
		}
		rows, err := repo.db.QueryContext(ctx, q, dummyArgs...)
		if err != nil {
			t.Logf("EXPLAIN %s ERROR: %v", name, err)
			continue
		}
		t.Logf("=== EXPLAIN %s ===", name)
		for rows.Next() {
			var id, parent, notused int
			var detail string
			if err := rows.Scan(&id, &parent, &notused, &detail); err == nil {
				t.Logf("  %s", detail)
			}
		}
		rows.Close()
	}
}

func TestNotificationRepository(t *testing.T) {
	// 使用共享内存数据库 DSN
	repo, cleanup, err := NewNotificationRepository("file::memory:?mode=memory&cache=shared", notification.NewFilterBuilder())
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

	// Save 时未到 notBefore，该频道尚无可见通知，符合 GraphQL NonNull 规范不应该返回
	channels = nil
	for ch, err := range repo.Channels(ctx) {
		require.NoError(t, err)
		if ch.Channel == "hook:pending" {
			channels = append(channels, ch)
		}
	}
	assert.Len(t, channels, 0)

	// 等待 15ms 使 notBefore 到达，再次调用 Channels 触发被动增量刷新
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

	// 7. 测试 reclaim 物理清理过期的旧通知
	nExpired, err := notification.FromRepository(
		scalar.ToID("notif:44444444-4444-4444-4444-444444444444"),
		"44444444-4444-4444-4444-444444444444",
		"hook:expired",
		"Expired Title",
		"Expired Body",
		shared.NotificationPriorityNormal,
		time.Time{},
		time.Time{},
		time.Now().Add(-100*24*time.Hour), // notAfter: 100 天前已过期
		time.Now().Add(-101*24*time.Hour),
		time.Now().Add(-101*24*time.Hour),
		time.Now().Add(-101*24*time.Hour),
		scalar.URI{},
	)
	require.NoError(t, err)
	_, err = repo.Save(ctx, nExpired)
	require.NoError(t, err)

	err = repo.reclaim(30 * 24 * time.Hour)
	require.NoError(t, err)

	_, err = repo.Get(ctx, "notif:44444444-4444-4444-4444-444444444444")
	assert.Error(t, err)
}