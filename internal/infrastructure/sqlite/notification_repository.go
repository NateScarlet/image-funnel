package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"iter"
	"math/rand/v2"
	"strings"
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
	sf            singleflight.Group
}

// notificationRepositoryOptions 配置选项，不可变
type notificationRepositoryOptions struct {
	reclaimGracePeriod  time.Duration
	reclaimErrorHandler func(error)
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

// NewNotificationRepository 实例化 SQLite 仓库
// 返回仓库实例和清理函数作为第二个返回值
func NewNotificationRepository(dbPath string, filterBuilder *notification.FilterBuilder, opts ...NotificationRepositoryOption) (*NotificationRepository, func() error, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open sqlite db: %w", err)
	}

	// 开启 WAL 模式以提升并发度
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("enable WAL mode: %w", err)
	}

	// 单连接确保写操作天然序列化，避免 SQLITE_BUSY
	db.SetMaxOpenConns(1)

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
			read_at INTEGER NOT NULL DEFAULT -62135596800000,
			dismissed_at INTEGER NOT NULL DEFAULT -62135596800000,
			not_after INTEGER NOT NULL,
			not_before INTEGER NOT NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			details_url TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_notifications_channel_created ON notifications(channel, created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_notifications_tag ON notifications(tag);

		CREATE TABLE IF NOT EXISTS channel_summary (
			channel TEXT PRIMARY KEY,
			unread_count INTEGER NOT NULL DEFAULT 0,
			latest_notification_id TEXT NOT NULL,
			expires_at INTEGER NOT NULL DEFAULT 0
		);
		PRAGMA user_version = 1;
		`
		if _, err := db.Exec(schema); err != nil {
			db.Close()
			return nil, nil, fmt.Errorf("init notifications schema: %w", err)
		}
	}

	repo := &NotificationRepository{
		db:            db,
		filterBuilder: filterBuilder,
	}

	// 应用选项（不可变模式：仅在初始化时读取一次）
	o := &notificationRepositoryOptions{
		reclaimGracePeriod: 30 * 24 * time.Hour,
	}
	for _, opt := range opts {
		opt(o)
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

// Save 保存通知（新建或更新），返回是否为新建通知
func (r *NotificationRepository) Save(ctx context.Context, notif *notification.Notification) (didCreate bool, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	// 检查 tag 是否已经存在
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
	`
	po := newNotificationPO(notif)
	_, err = tx.ExecContext(ctx, query,
		po.ID, po.Tag, po.Channel, po.Title, po.Body, po.Priority,
		po.ReadAt, po.DismissedAt, po.NotAfter, po.NotBefore,
		po.CreatedAt, po.UpdatedAt, po.DetailsURL,
	)
	if err != nil {
		return false, err
	}

	// 刷新该频道物化视图（写时维护，避免 Channels 的 N+1）
	if err := r.refreshChannelSummary(ctx, tx, notif.Channel()); err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}

	return didCreate, nil
}

// Get 根据 ID 获取通知，不存在返回 apperror.NewErrDocumentNotFound
func (r *NotificationRepository) Get(ctx context.Context, id string) (*notification.Notification, error) {
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
	notif, err := scanNotificationRow(r.db.QueryRowContext(ctx, `
		SELECT id, tag, channel, title, body, priority, read_at, dismissed_at, not_after, not_before, created_at, updated_at, details_url
		FROM notifications WHERE tag = ?
		`, tag))
		if err == sql.ErrNoRows {
			return nil, apperror.NewErrDocumentNotFound(scalar.ToID(tag))
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
			query += " AND not_after >= ?"
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

// Channels 遍历获取所有频道统计信息（读物化视图，单查询无 N+1）
// rows 扫描完成后若检测到有 expires_at 过期，关闭 rows 后通过 singleflight 精准增量刷新相应频道
func (r *NotificationRepository) Channels(ctx context.Context) iter.Seq2[*notification.ChannelStats, error] {
	return func(yield func(*notification.ChannelStats, error) bool) {
		rows, err := r.db.QueryContext(ctx, `
			SELECT channel, unread_count, latest_notification_id, expires_at
			FROM channel_summary
			ORDER BY channel
		`)
		if err != nil {
			yield(nil, err)
			return
		}

		type summaryItem struct {
			cs        notification.ChannelStats
			expiresAt int64
		}
		var items []summaryItem

		for rows.Next() {
			var (
				item                 summaryItem
				latestNotificationID string
			)
			err := rows.Scan(&item.cs.Channel, &item.cs.UnreadCount, &latestNotificationID, &item.expiresAt)
			if err != nil {
				rows.Close()
				yield(nil, err)
				return
			}
			item.cs.LatestNotificationID = scalar.ToID(latestNotificationID)
			items = append(items, item)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			yield(nil, err)
			return
		}
		rows.Close()

		now := time.Now().UnixMilli()

		for _, item := range items {
			cs := item.cs
			// 被动刷新：若该频道 expires_at 已到/已过期，通过 singleflight 精准触发该频道的增量刷新
			if item.expiresAt > 0 && item.expiresAt <= now {
				res, err, _ := r.sf.Do(cs.Channel, func() (any, error) {
					return r.refreshChannel(ctx, cs.Channel)
				})
				if err != nil {
					if !yield(nil, err) {
						return
					}
					continue
				}
				updatedStats, ok := res.(*notification.ChannelStats)
				if !ok || updatedStats == nil {
					// 频道无有效通知已被删除
					continue
				}
				cs = *updatedStats
			}

			if !yield(&cs, nil) {
				return
			}
		}
	}
}

// refreshChannel 在独立的单键 singleflight 中增量刷新并返回特定频道的摘要信息
// 如果刷新后该频道没有通知（已删除），返回 (nil, nil)
func (r *NotificationRepository) refreshChannel(ctx context.Context, channel string) (*notification.ChannelStats, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("refreshChannel BeginTx %s: %w", channel, err)
	}
	defer tx.Rollback()

	if err := r.refreshChannelSummary(ctx, tx, channel); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("refreshChannel Commit %s: %w", channel, err)
	}

	// 读取更新后的频道摘要
	var (
		cs                   notification.ChannelStats
		latestNotificationID string
	)
	cs.Channel = channel
	err = r.db.QueryRowContext(ctx, `
		SELECT unread_count, latest_notification_id
		FROM channel_summary
		WHERE channel = ?
	`, channel).Scan(&cs.UnreadCount, &latestNotificationID)
	if err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("read updated channel summary %s: %w", channel, err)
	}

	cs.LatestNotificationID = scalar.ToID(latestNotificationID)
	return &cs, nil
}

// refreshChannelSummary 在写事务内用标量子查询刷新频道物化视图
func (r *NotificationRepository) refreshChannelSummary(ctx context.Context, tx *sql.Tx, channel string) error {
	now := time.Now().UnixMilli()

	// 先写入或更新摘要
	if _, err := tx.ExecContext(ctx, `
		INSERT OR REPLACE INTO channel_summary (channel, unread_count, latest_notification_id, expires_at)
		SELECT ?,
		       (SELECT COUNT(*) FROM notifications n WHERE n.channel = ? AND n.read_at = ?),
		       -- latestNotification 语义：当前可见通知（not_before <= now）中创建时间最新的
		       COALESCE((SELECT n.id FROM notifications n WHERE n.channel = ? AND n.not_before <= ? ORDER BY n.created_at DESC, n.id DESC LIMIT 1), ''),
		       -- expires_at：下一条待发送通知的 notBefore，没有则 0 永不过期
		       COALESCE((SELECT MIN(n.not_before) FROM notifications n WHERE n.channel = ? AND n.not_before > ?), 0)
		WHERE (SELECT COUNT(*) FROM notifications WHERE channel = ?) > 0
	`, channel, channel, zeroTimeMs, channel, now, channel, now, channel); err != nil {
		return fmt.Errorf("refreshChannelSummary upsert channel %s: %w", channel, err)
	}

	// 再删除已空频道（该频道所有通知已被物理删除）
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM channel_summary WHERE channel = ? AND (
			SELECT COUNT(*) FROM notifications WHERE channel = ?
		) = 0
	`, channel, channel); err != nil {
		return fmt.Errorf("refreshChannelSummary delete empty channel %s: %w", channel, err)
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
	cutoff := time.Now().Add(-reclaimGracePeriod).UnixMilli()

	// 删除过期通知以及对应的 channel_summary
	if _, err := r.db.Exec(`
		DELETE FROM channel_summary WHERE channel IN (
			SELECT channel FROM notifications WHERE not_after <= ?
		)
	`, cutoff); err != nil {
		return fmt.Errorf("reclaim channel_summary: %w", err)
	}

	if _, err := r.db.Exec(`
		DELETE FROM notifications WHERE not_after <= ?
	`, cutoff); err != nil {
		return fmt.Errorf("reclaim notifications: %w", err)
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