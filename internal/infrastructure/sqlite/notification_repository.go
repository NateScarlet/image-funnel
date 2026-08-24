package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"iter"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"time"

	"main/internal/apperror"
	"main/internal/domain/notification"
	"main/internal/enum"
	"main/internal/scalar"
	"main/internal/shared"

	"golang.org/x/sync/singleflight"
	_ "modernc.org/sqlite"
)

// 编译时接口检查
var _ notification.Repository = (*NotificationRepository)(nil)

// zeroTimeMs time.Time{} 对应的 unix 毫秒值，用于数据库中表示未设置的时间
const zeroTimeMs int64 = -62135596800000

// NotificationRepository 基于 SQLite 的通知存储仓库
type NotificationRepository struct {
	db            *sql.DB
	filterBuilder *notification.FilterBuilder
	nowFunc       func() time.Time
	sf            singleflight.Group
	writeMu       sync.Mutex
}

// notificationRepositoryOptions 配置选项，不可变
type notificationRepositoryOptions struct {
	reclaimGracePeriod  time.Duration
	reclaimErrorHandler func(error)
	now                 func() time.Time
}

// NotificationRepositoryOption 配置选项
type NotificationRepositoryOption func(*notificationRepositoryOptions)

// WithReclaimGracePeriod 设置过期数据清理前的等待时间，默认 30 天
func WithReclaimGracePeriod(d time.Duration) NotificationRepositoryOption {
	return func(o *notificationRepositoryOptions) {
		o.reclaimGracePeriod = d
	}
}

// WithReclaimErrorHandler 设置后台清理错误的处理函数，默认丢弃
func WithReclaimErrorHandler(h func(error)) NotificationRepositoryOption {
	return func(o *notificationRepositoryOptions) {
		o.reclaimErrorHandler = h
	}
}

// WithNow 设置获取当前时间的函数（默认为 time.Now），主要用于单元测试注入当前时间
func WithNow(f func() time.Time) NotificationRepositoryOption {
	return func(o *notificationRepositoryOptions) {
		o.now = f
	}
}

// NewNotificationRepository 实例化 SQLite 仓库，直接传入 dsn 字符串
// 返回仓库实例和清理函数作为第二个返回值
func NewNotificationRepository(dsn string, filterBuilder *notification.FilterBuilder, opts ...NotificationRepositoryOption) (*NotificationRepository, func() error, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("open sqlite db: %w", err)
	}

	// 开启 WAL 模式与 busy_timeout 以提升并发读写性能
	if _, err := db.Exec("PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;"); err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("enable WAL mode: %w", err)
	}

	// user_version 检查：当前版本为 1，拒绝更高版本
	var userVersion int
	if err := db.QueryRow("PRAGMA user_version").Scan(&userVersion); err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("read user_version: %w", err)
	}
	if userVersion > 1 {
		db.Close()
		return nil, nil, fmt.Errorf("database version %d is higher than supported (1)", userVersion)
	}

	// 初始化数据表及索引
	if userVersion < 1 {
		schema := `
		CREATE TABLE IF NOT EXISTS notifications (
			id TEXT PRIMARY KEY,
			tag TEXT UNIQUE NOT NULL,
			channel TEXT NOT NULL,
			title TEXT NOT NULL,
			body TEXT NOT NULL,
			priority TEXT NOT NULL,
			read_at INTEGER NOT NULL,
			dismissed_at INTEGER NOT NULL,
			not_after INTEGER NOT NULL,
			not_before INTEGER NOT NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			details_url TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_notifications_channel_not_before ON notifications(channel, not_before DESC, id DESC);
		CREATE INDEX IF NOT EXISTS idx_notifications_not_after ON notifications(not_after);
		CREATE INDEX IF NOT EXISTS idx_notifications_tag ON notifications(tag);

		CREATE TABLE IF NOT EXISTS channel_summary (
			channel TEXT PRIMARY KEY,
			unread_count INTEGER NOT NULL,
			latest_notification_id TEXT NOT NULL,
			latest_not_before INTEGER NOT NULL,
			expires_at INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_channel_summary_latest_not_before ON channel_summary(latest_not_before DESC);
		PRAGMA user_version = 1;
		`
		if _, err := db.Exec(schema); err != nil {
			db.Close()
			return nil, nil, fmt.Errorf("init notifications schema: %w", err)
		}
	}

	// 应用选项（不可变模式：仅在初始化时读取一次）
	o := &notificationRepositoryOptions{
		reclaimGracePeriod: 30 * 24 * time.Hour,
		now:                time.Now,
	}
	for _, opt := range opts {
		opt(o)
	}
	if o.now == nil {
		o.now = time.Now
	}

	repo := &NotificationRepository{
		db:            db,
		filterBuilder: filterBuilder,
		nowFunc:       o.now,
	}

	// 创建者负责清理：通过 cancel 中止后台 goroutine
	cleanupCtx, cancelCleanup := context.WithCancel(context.Background())
	go repo.runCleanup(cleanupCtx, o.reclaimGracePeriod, o.reclaimErrorHandler)

	cleanup := func() error {
		cancelCleanup()
		return db.Close()
	}

	return repo, cleanup, nil
}

// Save 保存通知（新建或更新），返回是否为新建通知。
// 版本冲突（更新的 updated_at 早于数据库中的版本）时返回 VERSION_CONFLICT 错误。
func (r *NotificationRepository) Save(ctx context.Context, notif *notification.Notification) (didCreate bool, err error) {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	// 检查 tag 是否已经存在
	// EXPLAIN QUERY PLAN:
	// SEARCH notifications USING COVERING INDEX sqlite_autoindex_notifications_2 (tag=?)
	var count int
	err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM notifications WHERE tag = ?", notif.Tag()).Scan(&count)
	if err != nil {
		return false, err
	}
	didCreate = count == 0

	query := `
	INSERT INTO notifications (id, tag, channel, title, body, priority, read_at, dismissed_at, not_after, not_before, created_at, updated_at, details_url)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(tag) DO UPDATE SET
		title = excluded.title,
		body = excluded.body,
		priority = excluded.priority,
		read_at = excluded.read_at,
		dismissed_at = excluded.dismissed_at,
		not_after = excluded.not_after,
		not_before = excluded.not_before,
		updated_at = excluded.updated_at,
		details_url = excluded.details_url
	WHERE excluded.updated_at >= notifications.updated_at
	`
	po := newNotificationPO(notif)
	res, err := tx.ExecContext(ctx, query,
		po.ID, po.Tag, po.Channel, po.Title, po.Body, po.Priority,
		po.ReadAt, po.DismissedAt, po.NotAfter, po.NotBefore,
		po.CreatedAt, po.UpdatedAt, po.DetailsURL,
	)
	if err != nil {
		return false, err
	}

	// 对于更新操作，检测版本冲突：WHERE 条件不满足时未修改任何行
	if !didCreate {
		n, checkErr := res.RowsAffected()
		if checkErr == nil && n == 0 {
			return false, apperror.New(
				"VERSION_CONFLICT",
				fmt.Sprintf("notification with tag %q has newer version in database", po.Tag),
				fmt.Sprintf("通知 %q 已被其他进程修改", po.Tag),
			)
		}
	}

	// 刷新该频道物化视图（写时维护，避免 Channels 的 N+1）
	if err := r.refreshChannelSummaryTx(ctx, tx, notif.Channel()); err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}

	return didCreate, nil
}

// Get 根据 ID 获取通知，不存在返回 apperror.NewErrDocumentNotFound
func (r *NotificationRepository) Get(ctx context.Context, id string) (*notification.Notification, error) {
	// EXPLAIN QUERY PLAN:
	// SEARCH notifications USING INDEX sqlite_autoindex_notifications_1 (id=?)
	notif, err := scanNotificationRow(r.db.QueryRowContext(ctx, `
		SELECT id, tag, channel, title, body, priority, read_at, dismissed_at, not_after, not_before, created_at, updated_at, details_url
		FROM notifications WHERE id = ?
	`, id))
	if err == sql.ErrNoRows {
		return nil, apperror.NewErrDocumentNotFound(scalar.ToID(id))
	} else if err != nil {
		return nil, err
	}
	return notif, nil
}

// GetByTag 根据 tag 获取通知，不存在返回 apperror.NewErrDocumentNotFound
func (r *NotificationRepository) GetByTag(ctx context.Context, tag string) (*notification.Notification, error) {
	// EXPLAIN QUERY PLAN:
	// SEARCH notifications USING INDEX sqlite_autoindex_notifications_2 (tag=?)
	notif, err := scanNotificationRow(r.db.QueryRowContext(ctx, `
		SELECT id, tag, channel, title, body, priority, read_at, dismissed_at, not_after, not_before, created_at, updated_at, details_url
		FROM notifications WHERE tag = ?
		`, tag))
	if err == sql.ErrNoRows {
		return nil, apperror.New("NOT_FOUND", fmt.Sprintf("notification with tag %q not found", tag), fmt.Sprintf("未找到标签为 %q 的通知", tag))
	} else if err != nil {
		return nil, err
	}
	return notif, nil
}

// Find 遍历所有通知，利用 filter 粗筛（可索引加速），细筛由 FilterBuilder 兜底
func (r *NotificationRepository) Find(ctx context.Context, options ...notification.FindOption) iter.Seq2[*notification.Notification, error] {
	opts := notification.NewFindOptions(options...)
	filter := opts.Filter()
	filterFunc := r.filterBuilder.Build(filter)

	return func(yield func(*notification.Notification, error) bool) {
		query := `
		SELECT id, tag, channel, title, body, priority, read_at, dismissed_at, not_after, not_before, created_at, updated_at, details_url
		FROM notifications
		WHERE 1=1
		`
		var args []any
		if len(filter.Channel) > 0 {
			query += " AND channel IN (?" + strings.Repeat(",?", len(filter.Channel)-1) + ")"
			for _, ch := range filter.Channel {
				args = append(args, ch)
			}
		}
		if filter.Read != nil {
			if *filter.Read {
				query += " AND read_at != ?"
			} else {
				query += " AND read_at = ?"
			}
			args = append(args, zeroTimeMs)
		}
		if len(filter.Priority) > 0 {
			query += " AND priority IN (?" + strings.Repeat(",?", len(filter.Priority)-1) + ")"
			for _, p := range filter.Priority {
				args = append(args, p.String())
			}
		}
		if !filter.VisibleAt.IsZero() {
			t := filter.VisibleAt.UnixMilli()
			query += " AND not_before <= ?"
			args = append(args, t)
			query += " AND not_after > ?"
			args = append(args, t)
		}
		if !filter.PendingAt.IsZero() {
			// not_before > t：在时间点 t 尚未到达可见时间（未来才可见）
			query += " AND not_before > ?"
			args = append(args, filter.PendingAt.UnixMilli())
		}
		if !filter.ExpiredAt.IsZero() {
			// not_after < t：在时间点 t 已超出可见截止（已过期）
			query += " AND not_after < ?"
			args = append(args, filter.ExpiredAt.UnixMilli())
		}

		query += " ORDER BY created_at DESC, id DESC"

		// EXPLAIN QUERY PLAN:
		// SEARCH notifications USING INDEX idx_notifications_channel_not_before
		rows, err := r.db.QueryContext(ctx, query, args...)
		if err != nil {
			yield(nil, err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			notif, err := scanNotificationRow(rows)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			if !filterFunc(notif) {
				continue
			}

			if !yield(notif, nil) {
				return
			}
		}

		if err := rows.Err(); err != nil {
			yield(nil, err)
		}
	}
}

type refreshedChannelSummary struct {
	cs              *notification.ChannelStats
	latestNotBefore int64
}

func (r *NotificationRepository) now() time.Time {
	if r.nowFunc != nil {
		return r.nowFunc()
	}
	return time.Now()
}

// Channels 遍历获取所有频道统计信息（读物化视图，单查询无 N+1）
// 通过 SQL 排序优先返回过期项（ORDER BY CASE WHEN expires_at <= :now THEN 0 ELSE 1 END ASC）：
// 1. 无过期项时（绝大多数正常情况）：零内存分配，单循环纯流式直通输出；
// 2. 有过期项时：扫描并收集过期频道后关闭 rows 释放锁，通过 singleflight 增量刷新并与新鲜项交替归并输出。
func (r *NotificationRepository) Channels(ctx context.Context) iter.Seq2[*notification.ChannelStats, error] {
	return func(yield func(*notification.ChannelStats, error) bool) {
		now := r.now().UnixMilli()

		// EXPLAIN QUERY PLAN:
		// SCAN channel_summary
		// USE TEMP B-TREE FOR ORDER BY
		rows, err := r.db.QueryContext(ctx, `
			SELECT channel, unread_count, latest_notification_id, latest_not_before, expires_at
			FROM channel_summary
			ORDER BY
				CASE WHEN expires_at <= :now THEN 0 ELSE 1 END ASC,
				latest_not_before DESC,
				channel ASC
		`, sql.Named("now", now))
		if err != nil {
			yield(nil, err)
			return
		}

		var expiredChannels []string

		for rows.Next() {
			var (
				cs                   notification.ChannelStats
				latestNotificationID string
				latestNotBefore      int64
				expiresAt            int64
			)
			err := rows.Scan(&cs.Channel, &cs.UnreadCount, &latestNotificationID, &latestNotBefore, &expiresAt)
			if err != nil {
				rows.Close()
				yield(nil, err)
				return
			}

			if expiresAt <= now {
				// 收集在结果集开头的过期频道（无论此前是否有可见通知，到期必须刷新）
				expiredChannels = append(expiredChannels, cs.Channel)
				continue
			}

			// 遇到第一个新鲜项！
			if len(expiredChannels) > 0 {
				// 存在过期项：先关闭当前 rows 释放锁，再执行单键增量刷新与归并输出
				rows.Close()
				r.yieldChannelsWithExpired(ctx, yield, expiredChannels, &cs, latestNotificationID, latestNotBefore, now)
				return
			}

			// 无过期项场景：单循环直通输出当前及后续新鲜项（过滤无可见通知频道）
			if latestNotificationID == "" {
				continue
			}
			cs.LatestNotificationID = scalar.ToID(latestNotificationID)
			if !yield(&cs, nil) {
				rows.Close()
				return
			}
		}

		if err := rows.Err(); err != nil {
			rows.Close()
			yield(nil, err)
			return
		}
		rows.Close()

		// 表中全都是过期项
		if len(expiredChannels) > 0 {
			r.yieldChannelsWithExpired(ctx, yield, expiredChannels, nil, "", 0, now)
		}
	}
}

func (r *NotificationRepository) yieldChannelsWithExpired(
	ctx context.Context,
	yield func(*notification.ChannelStats, error) bool,
	expiredChannels []string,
	firstFreshCS *notification.ChannelStats,
	firstFreshLatestID string,
	firstFreshNotBefore int64,
	now int64,
) {
	refreshCtx := context.WithoutCancel(ctx)

	// 1. 刷新所有过期频道并收集结果
	var refreshed []*refreshedChannelSummary
	for _, ch := range expiredChannels {
		res, err, _ := r.sf.Do(ch, func() (any, error) {
			return r.refreshChannelSummary(refreshCtx, ch)
		})
		if err != nil {
			if !yield(nil, err) {
				return
			}
			continue
		}
		if rCS, ok := res.(*refreshedChannelSummary); ok && rCS != nil && rCS.cs != nil && rCS.cs.LatestNotificationID.String() != "" {
			refreshed = append(refreshed, rCS)
		}
	}

	// 按 latestNotBefore DESC, channel ASC 排序已刷新的过期项
	slices.SortFunc(refreshed, func(a, b *refreshedChannelSummary) int {
		if a.latestNotBefore != b.latestNotBefore {
			if a.latestNotBefore > b.latestNotBefore {
				return -1
			}
			return 1
		}
		return strings.Compare(a.cs.Channel, b.cs.Channel)
	})

	// 辅助闭包：输出所有 latestNotBefore >= targetNotBefore 的已刷新过期项
	yieldRefreshedGe := func(targetNotBefore int64, targetChannel string) bool {
		for len(refreshed) > 0 {
			item := refreshed[0]
			if item.latestNotBefore > targetNotBefore || (item.latestNotBefore == targetNotBefore && item.cs.Channel < targetChannel) {
				refreshed = refreshed[1:]
				if !yield(item.cs, nil) {
					return false
				}
			} else {
				break
			}
		}
		return true
	}

	// 2. 如果存在之前已经 scan 到的首个新鲜项，进行交替输出
	if firstFreshCS != nil && firstFreshLatestID != "" {
		if !yieldRefreshedGe(firstFreshNotBefore, firstFreshCS.Channel) {
			return
		}
		firstFreshCS.LatestNotificationID = scalar.ToID(firstFreshLatestID)
		if !yield(firstFreshCS, nil) {
			return
		}

		// 3. 继续流式查询后续所有新鲜项（仅查询有可见通知的频道）
		// EXPLAIN QUERY PLAN:
		// SCAN channel_summary USING INDEX idx_channel_summary_latest_not_before
		// USE TEMP B-TREE FOR LAST TERM OF ORDER BY
		freshRows, err := r.db.QueryContext(ctx, `
			SELECT channel, unread_count, latest_notification_id, latest_not_before
			FROM channel_summary
			WHERE expires_at > :now AND latest_notification_id != ''
			ORDER BY latest_not_before DESC, channel ASC
		`, sql.Named("now", now))
		if err != nil {
			yield(nil, err)
			return
		}
		defer freshRows.Close()

		skippedFirst := false
		for freshRows.Next() {
			var (
				itemCS               notification.ChannelStats
				latestNotificationID string
				latestNotBefore      int64
			)
			err := freshRows.Scan(&itemCS.Channel, &itemCS.UnreadCount, &latestNotificationID, &latestNotBefore)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			if !skippedFirst && itemCS.Channel == firstFreshCS.Channel {
				skippedFirst = true
				continue
			}
			// 步骤 1 刷新过期频道后，其在数据库里的 expires_at 已更新，
			// 此处跳过已在 expiredChannels 中由步骤 1 刷新的频道，避免重复输出。
			if slices.Contains(expiredChannels, itemCS.Channel) {
				continue
			}

			if !yieldRefreshedGe(latestNotBefore, itemCS.Channel) {
				return
			}
			itemCS.LatestNotificationID = scalar.ToID(latestNotificationID)
			if !yield(&itemCS, nil) {
				return
			}
		}
		if err := freshRows.Err(); err != nil {
			yield(nil, err)
			return
		}
	}

	// 4. 发送剩余所有已刷新的过期项
	for _, item := range refreshed {
		if !yield(item.cs, nil) {
			return
		}
	}
}

// refreshChannelSummary 在独立的单键 singleflight 中增量刷新并返回特定频道的摘要信息
// 如果刷新后该频道没有通知（已删除），返回 (nil, nil)
func (r *NotificationRepository) refreshChannelSummary(ctx context.Context, channel string) (*refreshedChannelSummary, error) {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("refreshChannelSummary BeginTx %s: %w", channel, err)
	}
	defer tx.Rollback()

	if err := r.refreshChannelSummaryTx(ctx, tx, channel); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("refreshChannelSummary Commit %s: %w", channel, err)
	}

	// 读取更新后的频道摘要
	// EXPLAIN QUERY PLAN:
	// SEARCH channel_summary USING INDEX sqlite_autoindex_channel_summary_1 (channel=?)
	var (
		cs                   notification.ChannelStats
		latestNotificationID string
		latestNotBefore      int64
	)
	err = r.db.QueryRowContext(ctx, `
		SELECT channel, unread_count, latest_notification_id, latest_not_before
		FROM channel_summary
		WHERE channel = ?
	`, channel).Scan(&cs.Channel, &cs.UnreadCount, &latestNotificationID, &latestNotBefore)
	if err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("read updated channel summary %s: %w", channel, err)
	}

	cs.LatestNotificationID = scalar.ToID(latestNotificationID)
	return &refreshedChannelSummary{cs: &cs, latestNotBefore: latestNotBefore}, nil
}

// refreshChannelSummaryTx 在写事务内用纯 SQL 标量子查询与 CTE 更新特定频道的物化视图摘要
// 如果刷新后该频道既无可见通知又无未来等待发送的事件，将其从 channel_summary 中彻底删除
func (r *NotificationRepository) refreshChannelSummaryTx(ctx context.Context, tx *sql.Tx, channel string) error {
	now := r.now().UnixMilli()

	// 1. 若频道既无可见通知又无 pending 通知，物理清理其在 channel_summary 中的摘要
	// EXPLAIN QUERY PLAN:
	// SEARCH channel_summary USING INDEX sqlite_autoindex_channel_summary_1 (channel=?)
	// SCALAR SUBQUERY 1: SEARCH notifications USING INDEX idx_notifications_channel_not_before (channel=? AND not_before<?)
	// SCALAR SUBQUERY 2: SEARCH notifications USING COVERING INDEX idx_notifications_channel_not_before (channel=? AND not_before>?)
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM channel_summary
		WHERE channel = :channel
		  AND (SELECT COUNT(*) FROM notifications WHERE channel = :channel AND not_before <= :now AND not_after > :now) = 0
		  AND (SELECT COUNT(*) FROM notifications WHERE channel = :channel AND not_before > :now) = 0
	`, sql.Named("channel", channel), sql.Named("now", now)); err != nil {
		return fmt.Errorf("refreshChannelSummaryTx delete channel %s: %w", channel, err)
	}

	// 2. 纯 SQL 计算并写入/更新摘要（单次 CTE 子查询获取最新通知 id 与 not_before，使用命名参数消除重复）
	// EXPLAIN QUERY PLAN:
	// SCAN CONSTANT ROW
	// MATERIALIZE visible: SEARCH notifications USING INDEX idx_notifications_channel_not_before (channel=? AND not_before<?)
	// SCAN visible
	// SCALAR SUBQUERY 1: SEARCH notifications USING COVERING INDEX idx_notifications_channel_not_before (channel=? AND not_before>?)
	// SCALAR SUBQUERY 2: SEARCH notifications USING INDEX idx_notifications_channel_not_before (channel=? AND not_before<?)
	// SCALAR SUBQUERY 3: SCAN visible
	// SCALAR SUBQUERY 4: SCAN visible
	// SCALAR SUBQUERY 7: CO-ROUTINE (subquery-6)
	//   COMPOUND QUERY
	//   LEFT-MOST SUBQUERY: SEARCH notifications USING COVERING INDEX idx_notifications_channel_not_before (channel=? AND not_before>?)
	//   UNION ALL: SEARCH notifications USING INDEX idx_notifications_channel_not_before (channel=? AND not_before<?)
	if _, err := tx.ExecContext(ctx, `
		WITH visible AS (
			SELECT id, not_before
			FROM notifications
			WHERE channel = :channel AND not_before <= :now AND not_after > :now
			ORDER BY not_before DESC, id DESC
			LIMIT 1
		)
		INSERT OR REPLACE INTO channel_summary (channel, unread_count, latest_notification_id, latest_not_before, expires_at)
		SELECT
			:channel,
			(SELECT COUNT(*) FROM notifications WHERE channel = :channel AND read_at = :zeroTimeMs AND not_before <= :now AND not_after > :now),
			COALESCE((SELECT id FROM visible), ''),
			COALESCE((SELECT not_before FROM visible), 0),
			COALESCE((
				SELECT MIN(t) FROM (
					SELECT not_before AS t FROM notifications WHERE channel = :channel AND not_before > :now
					UNION ALL
					SELECT not_after AS t FROM notifications WHERE channel = :channel AND not_before <= :now AND not_after > :now
				)
			), 0)
		WHERE (SELECT COUNT(*) FROM visible) > 0
		   OR (SELECT COUNT(*) FROM notifications WHERE channel = :channel AND not_before > :now) > 0
	`,
		sql.Named("channel", channel),
		sql.Named("now", now),
		sql.Named("zeroTimeMs", zeroTimeMs),
	); err != nil {
		return fmt.Errorf("refreshChannelSummaryTx upsert channel %s: %w", channel, err)
	}

	return nil
}

// #region 后台清理

// runCleanup 后台 goroutine，随机延迟 0-1 小时后每 24 小时清理一次过期数据
func (r *NotificationRepository) runCleanup(ctx context.Context, reclaimGracePeriod time.Duration, reclaimErrorHandler func(error)) {
	// 随机延迟 0-1 小时
	initialDelay := time.Duration(rand.Int64N(int64(time.Hour)))
	select {
	case <-time.After(initialDelay):
	case <-ctx.Done():
		return
	}

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		if err := r.reclaim(reclaimGracePeriod); err != nil && reclaimErrorHandler != nil {
			reclaimErrorHandler(err)
		}

		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

// reclaim 清理 notAfter + reclaimGracePeriod 在当前时间之前的数据
func (r *NotificationRepository) reclaim(reclaimGracePeriod time.Duration) error {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()

	ctx := context.Background()
	cutoff := r.now().Add(-reclaimGracePeriod).UnixMilli()

	// EXPLAIN QUERY PLAN:
	// SEARCH notifications USING INDEX idx_notifications_not_after (not_after<?)
	if _, err := r.db.ExecContext(ctx, `
		DELETE FROM notifications WHERE not_after <= :cutoff
	`, sql.Named("cutoff", cutoff)); err != nil {
		return fmt.Errorf("reclaim delete notifications: %w", err)
	}

	return nil
}

// #endregion

// #region 辅助方法

// rowScanner 用于统一 *sql.Row 与 *sql.Rows 的 Scan 方法，以便 scanNotificationRow 复用单行扫描逻辑
type rowScanner interface {
	Scan(dest ...any) error
}

// notificationPO 通知的持久化对象，时间字段使用 unix 毫秒时间戳
type notificationPO struct {
	ID          string
	Tag         string
	Channel     string
	Title       string
	Body        string
	Priority    string
	ReadAt      int64
	DismissedAt int64
	NotAfter    int64
	NotBefore   int64
	CreatedAt   int64
	UpdatedAt   int64
	DetailsURL  string
}

func newNotificationPO(n *notification.Notification) *notificationPO {
	timeToMs := func(t time.Time) int64 { return t.UnixMilli() }
	return &notificationPO{
		ID:          n.ID().String(),
		Tag:         n.Tag(),
		Channel:     n.Channel(),
		Title:       n.Title(),
		Body:        n.Body(),
		Priority:    n.Priority().String(),
		ReadAt:      timeToMs(n.ReadAt()),
		DismissedAt: timeToMs(n.DismissedAt()),
		NotAfter:    timeToMs(n.NotAfter()),
		NotBefore:   timeToMs(n.NotBefore()),
		CreatedAt:   timeToMs(n.CreatedAt()),
		UpdatedAt:   timeToMs(n.UpdatedAt()),
		DetailsURL:  n.DetailsURL().String(),
	}
}

func (po *notificationPO) DomainObject() (*notification.Notification, error) {
	priority, err := enum.Parse[shared.NotificationPriorityMeta](po.Priority)
	if err != nil {
		return nil, fmt.Errorf("parse notification priority: %w", err)
	}

	detailsURL, err := scalar.ParseURI(po.DetailsURL)
	if err != nil {
		return nil, fmt.Errorf("parse notification detailsURL: %w", err)
	}

	msToTime := func(ms int64) time.Time { return time.UnixMilli(ms) }

	return notification.FromRepository(
		scalar.ToID(po.ID), po.Tag, po.Channel, po.Title, po.Body, priority,
		msToTime(po.ReadAt),
		msToTime(po.DismissedAt),
		msToTime(po.NotAfter),
		msToTime(po.NotBefore),
		msToTime(po.CreatedAt),
		msToTime(po.UpdatedAt),
		detailsURL,
	)
}

// scanNotificationRow 从 sql.Row 或 sql.Rows 扫描一行并构造领域对象
func scanNotificationRow(s rowScanner) (*notification.Notification, error) {
	var po notificationPO
	err := s.Scan(
		&po.ID, &po.Tag, &po.Channel, &po.Title, &po.Body, &po.Priority,
		&po.ReadAt, &po.DismissedAt, &po.NotAfter, &po.NotBefore,
		&po.CreatedAt, &po.UpdatedAt, &po.DetailsURL,
	)
	if err != nil {
		return nil, err
	}

	return po.DomainObject()
}

// #endregion
