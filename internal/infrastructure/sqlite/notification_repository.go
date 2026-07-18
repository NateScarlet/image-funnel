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

	_ "modernc.org/sqlite"
)

// 编译时接口检查
var _ notification.Repository = (*NotificationRepository)(nil)

// NotificationRepository 基于 SQLite 的通知存储仓库
type NotificationRepository struct {
	db                 *sql.DB
	reclaimGracePeriod time.Duration
	reclaimErrorHandler func(error)
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
func NewNotificationRepository(dbPath string, opts ...NotificationRepositoryOption) (*NotificationRepository, func() error, error) {
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
			read_at TEXT,
			dismissed_at TEXT,
			not_after TEXT,
			not_before TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			details_url TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_notifications_channel_created ON notifications(channel, created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_notifications_tag ON notifications(tag);

		CREATE TABLE IF NOT EXISTS channel_summary (
			channel TEXT PRIMARY KEY,
			unread_count INTEGER NOT NULL DEFAULT 0,
			latest_notification_id TEXT NOT NULL
		);
		PRAGMA user_version = 1;
		`
		if _, err := db.Exec(schema); err != nil {
			db.Close()
			return nil, nil, fmt.Errorf("init notifications schema: %w", err)
		}
	}

	repo := &NotificationRepository{
		db:                 db,
		reclaimGracePeriod: 30 * 24 * time.Hour,
	}

	// 应用选项（不可变模式：仅在初始化时读取一次）
	o := &notificationRepositoryOptions{
		reclaimGracePeriod: 30 * 24 * time.Hour,
	}
	for _, opt := range opts {
		opt(o)
	}
	repo.reclaimGracePeriod = o.reclaimGracePeriod
	repo.reclaimErrorHandler = o.reclaimErrorHandler

	// 创建者负责清理：通过 cancel 中止后台 goroutine
	cleanupCtx, cancelCleanup := context.WithCancel(context.Background())
	go repo.runCleanup(cleanupCtx)

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
	_, err = tx.ExecContext(ctx, query,
		notif.ID().String(),
		notif.Tag(),
		notif.Channel(),
		notif.Title(),
		notif.Body(),
		notif.Priority().String(),
		formatTime(notif.ReadAt()),
		formatTime(notif.DismissedAt()),
		formatTime(notif.NotAfter()),
		formatTime(notif.NotBefore()),
		formatTime(notif.CreatedAt()),
		formatTime(notif.UpdatedAt()),
		notif.DetailsURL().String(),
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
				query += " AND (read_at IS NOT NULL AND read_at != '')"
			} else {
				query += " AND (read_at IS NULL OR read_at = '')"
			}
		}
		if len(filter.Priority) > 0 {
			query += " AND priority IN (?" + strings.Repeat(",?", len(filter.Priority)-1) + ")"
			for _, p := range filter.Priority {
				args = append(args, p.String())
			}
		}
		if filter.VisibleAt != nil {
			t := filter.VisibleAt.Format(time.RFC3339Nano)
			query += " AND (not_before IS NULL OR not_before = '' OR not_before <= ?)"
			args = append(args, t)
			query += " AND (not_after IS NULL OR not_after = '' OR not_after >= ?)"
			args = append(args, t)
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
func (r *NotificationRepository) Channels(ctx context.Context) iter.Seq2[*notification.ChannelStats, error] {
	return func(yield func(*notification.ChannelStats, error) bool) {
		rows, err := r.db.QueryContext(ctx, `
			SELECT channel, unread_count, latest_notification_id
			FROM channel_summary
			ORDER BY channel
		`)
		if err != nil {
			yield(nil, err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var (
				cs                  notification.ChannelStats
				latestNotificationID string
			)
			err := rows.Scan(&cs.Channel, &cs.UnreadCount, &latestNotificationID)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			cs.LatestNotificationID = scalar.ToID(latestNotificationID)
			if !yield(&cs, nil) {
				return
			}
		}

		if err := rows.Err(); err != nil {
			yield(nil, err)
		}
	}
}

// refreshChannelSummary 在写事务内用标量子查询刷新频道物化视图
func (r *NotificationRepository) refreshChannelSummary(ctx context.Context, tx *sql.Tx, channel string) error {
	// 删除已空频道（该频道所有通知已被物理删除）
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM channel_summary WHERE channel = ? AND (
			SELECT COUNT(*) FROM notifications WHERE channel = ?
		) = 0
	`, channel, channel); err != nil {
		return fmt.Errorf("refreshChannelSummary delete empty channel %s: %w", channel, err)
	}

	// 仅在有通知时写入或更新摘要，避免插入空行
	if _, err := tx.ExecContext(ctx, `
		INSERT OR REPLACE INTO channel_summary (channel, unread_count, latest_notification_id)
		SELECT ?,
		       (SELECT COUNT(*) FROM notifications n WHERE n.channel = ? AND (n.read_at IS NULL OR n.read_at = '')),
		       (SELECT n.id FROM notifications n WHERE n.channel = ? ORDER BY n.created_at DESC, n.id DESC LIMIT 1)
		WHERE (SELECT COUNT(*) FROM notifications WHERE channel = ?) > 0
	`, channel, channel, channel, channel); err != nil {
		return fmt.Errorf("refreshChannelSummary upsert channel %s: %w", channel, err)
	}

	return nil
}

// #region 后台清理

// runCleanup 后台 goroutine，随机延迟 0-1 小时后每 24 小时清理一次过期数据
func (r *NotificationRepository) runCleanup(ctx context.Context) {
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
		r.reclaim()

		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

// reclaim 清理 notAfter + reclaimGracePeriod 在当前时间之前的数据
func (r *NotificationRepository) reclaim() {
	cutoff := time.Now().Add(-r.reclaimGracePeriod).Format(time.RFC3339Nano)

	// 删除过期通知以及对应的 channel_summary
	_, err := r.db.Exec(`
		DELETE FROM channel_summary WHERE channel IN (
			SELECT channel FROM notifications WHERE not_after != '' AND not_after <= ?
		)
	`, cutoff)
	if err != nil && r.reclaimErrorHandler != nil {
		r.reclaimErrorHandler(err)
	}

	_, err = r.db.Exec(`
		DELETE FROM notifications WHERE not_after != '' AND not_after <= ?
	`, cutoff)
	if err != nil && r.reclaimErrorHandler != nil {
		r.reclaimErrorHandler(err)
	}
}

// #endregion

// #region 辅助方法

type rowScanner interface {
	Scan(dest ...any) error
}

// parseNotificationRow 从已扫描的字符串字段构造领域对象
func parseNotificationRow(
	id, tag, channel, title, body, priorityStr string,
	readAtStr, dismissedAtStr, notAfterStr, notBeforeStr string,
	createdAtStr, updatedAtStr, detailsURLStr string,
) (*notification.Notification, error) {
	priority, err := enum.Parse[shared.NotificationPriorityMeta](priorityStr)
	if err != nil {
		return nil, fmt.Errorf("parse notification priority: %w", err)
	}

	readAt, err := parseTime(readAtStr)
	if err != nil {
		return nil, err
	}
	dismissedAt, err := parseTime(dismissedAtStr)
	if err != nil {
		return nil, err
	}
	notAfter, err := parseTime(notAfterStr)
	if err != nil {
		return nil, err
	}
	notBefore, err := parseTime(notBeforeStr)
	if err != nil {
		return nil, err
	}
	createdAt, err := parseTime(createdAtStr)
	if err != nil {
		return nil, err
	}
	updatedAt, err := parseTime(updatedAtStr)
	if err != nil {
		return nil, err
	}
	detailsURL, err := scalar.ParseURI(detailsURLStr)
	if err != nil {
		return nil, fmt.Errorf("parse notification detailsURL: %w", err)
	}

	return notification.FromRepository(
			scalar.ToID(id), tag, channel, title, body, priority,
			readAt, dismissedAt, notAfter, notBefore,
			createdAt, updatedAt, detailsURL,
		)
}

// scanNotificationRow 从 sql.Row 或 sql.Rows 扫描一行并构造领域对象
func scanNotificationRow(s rowScanner) (*notification.Notification, error) {
	var id, tag, channel, title, body, priorityStr, readAtStr, dismissedAtStr, notAfterStr, notBeforeStr, createdAtStr, updatedAtStr, detailsURLStr string
	err := s.Scan(
		&id, &tag, &channel, &title, &body, &priorityStr, &readAtStr, &dismissedAtStr, &notAfterStr, &notBeforeStr, &createdAtStr, &updatedAtStr, &detailsURLStr,
	)
	if err != nil {
		return nil, err
	}

	return parseNotificationRow(id, tag, channel, title, body, priorityStr,
		readAtStr, dismissedAtStr, notAfterStr, notBeforeStr,
		createdAtStr, updatedAtStr, detailsURLStr)
}

func parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse time string '%s': %w", s, err)
	}
	return t, nil
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339Nano)
}

// #endregion
