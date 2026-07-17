package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"time"

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
	db *sql.DB
}

// NewNotificationRepository 实例化 SQLite 仓库，如果目录不存在则创建
func NewNotificationRepository(dataDir string) (*NotificationRepository, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create sqlite data dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "notifications.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	// 开启 WAL 模式以提升并发度
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL mode: %w", err)
	}

	// 单连接确保写操作天然序列化，避免 SQLITE_BUSY
	db.SetMaxOpenConns(1)

	// 初始化数据表及索引（使用 CREATE TABLE IF NOT EXISTS 保证幂等）
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
		detail_url TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_notifications_channel_created ON notifications(channel, created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_notifications_tag ON notifications(tag);

	CREATE TABLE IF NOT EXISTS channel_summary (
		channel TEXT PRIMARY KEY,
		unread_count INTEGER NOT NULL DEFAULT 0,
		latest_notification_id TEXT NOT NULL,
		FOREIGN KEY (latest_notification_id) REFERENCES notifications(id)
	);
	`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("init notifications schema: %w", err)
	}

	return &NotificationRepository{db: db}, nil
}

// Close 释放数据库资源
func (r *NotificationRepository) Close() error {
	return r.db.Close()
}

// Save 保存通知（新建或更新），返回是否为新建通知
// 若 notif.IsDeleted() 为 true，则执行物理删除
func (r *NotificationRepository) Save(ctx context.Context, notif *notification.Notification) (didCreate bool, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	if notif.IsDeleted() {
		_, err = tx.ExecContext(ctx, "DELETE FROM notifications WHERE id = ?", notif.ID().String())
		if err != nil {
			return false, err
		}
	} else {
		// 检查 tag 是否已经存在
		var count int
		err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM notifications WHERE tag = ?", notif.Tag()).Scan(&count)
		if err != nil {
			return false, err
		}
		didCreate = count == 0

		query := `
		INSERT INTO notifications (id, tag, channel, title, body, priority, read_at, dismissed_at, not_after, not_before, created_at, updated_at, detail_url)
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
			detail_url = excluded.detail_url
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
			notif.DetailURL().String(),
		)
		if err != nil {
			return false, err
		}
	}

	// 刷新该频道物化视图（写时维护，避免 Channels 的 N+1）
	r.refreshChannelSummary(ctx, tx, notif.Channel())

	if err := tx.Commit(); err != nil {
		return false, err
	}

	return didCreate, nil
}

// Get 根据 ID 获取通知，不存在返回 nil, nil
func (r *NotificationRepository) Get(ctx context.Context, id string) (*notification.Notification, error) {
	notif, err := scanNotificationRow(r.db.QueryRowContext(ctx, `
		SELECT id, tag, channel, title, body, priority, read_at, dismissed_at, not_after, not_before, created_at, updated_at, detail_url
		FROM notifications WHERE id = ?
	`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return notif, nil
}

// GetByTag 根据 tag 获取通知，不存在返回 nil, nil
func (r *NotificationRepository) GetByTag(ctx context.Context, tag string) (*notification.Notification, error) {
	notif, err := scanNotificationRow(r.db.QueryRowContext(ctx, `
		SELECT id, tag, channel, title, body, priority, read_at, dismissed_at, not_after, not_before, created_at, updated_at, detail_url
		FROM notifications WHERE tag = ?
	`, tag))
	if err == sql.ErrNoRows {
		return nil, nil
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
		SELECT id, tag, channel, title, body, priority, read_at, dismissed_at, not_after, not_before, created_at, updated_at, detail_url
		FROM notifications
		WHERE 1=1
		`
		var args []any
		if filter.Channel != nil {
			query += " AND channel = ?"
			args = append(args, *filter.Channel)
		}
		if filter.Read != nil {
			if *filter.Read {
				query += " AND (read_at IS NOT NULL AND read_at != '' AND read_at != '0001-01-01T00:00:00Z')"
			} else {
				query += " AND (read_at IS NULL OR read_at = '' OR read_at = '0001-01-01T00:00:00Z')"
			}
		}
		if filter.Priority != nil {
			query += " AND priority = ?"
			args = append(args, filter.Priority.String())
		}
		if filter.VisibleAt != nil {
			t := filter.VisibleAt.Format(time.RFC3339Nano)
			query += " AND (not_before IS NULL OR not_before = '' OR not_before = '0001-01-01T00:00:00Z' OR not_before <= ?)"
			args = append(args, t)
			query += " AND (not_after IS NULL OR not_after = '' OR not_after = '0001-01-01T00:00:00Z' OR not_after >= ?)"
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
			SELECT cs.channel, cs.unread_count,
			       n.id, n.tag, n.channel, n.title, n.body, n.priority,
			       n.read_at, n.dismissed_at, n.not_after, n.not_before,
			       n.created_at, n.updated_at, n.detail_url
			FROM channel_summary cs
			LEFT JOIN notifications n ON n.id = cs.latest_notification_id
			ORDER BY cs.channel
		`)
		if err != nil {
			yield(nil, err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			cs, err := scanChannelSummaryRow(rows)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			if cs.LatestNotification == nil {
				continue // 对应通知已被物理删除，跳过
			}
			if !yield(cs, nil) {
				return
			}
		}

		if err := rows.Err(); err != nil {
			yield(nil, err)
		}
	}
}

// refreshChannelSummary 在写事务内用标量子查询刷新频道物化视图
func (r *NotificationRepository) refreshChannelSummary(ctx context.Context, tx *sql.Tx, channel string) {
	// 删除已空频道（该频道所有通知已被物理删除）
	_, _ = tx.ExecContext(ctx, `
		DELETE FROM channel_summary WHERE channel = ? AND (
			SELECT COUNT(*) FROM notifications WHERE channel = ?
		) = 0
	`, channel, channel)

	// 仅在有通知时写入或更新摘要，避免插入空行
	_, _ = tx.ExecContext(ctx, `
		INSERT OR REPLACE INTO channel_summary (channel, unread_count, latest_notification_id)
		SELECT ?,
		       (SELECT COUNT(*) FROM notifications n WHERE n.channel = ? AND (n.read_at IS NULL OR n.read_at = '' OR n.read_at = '0001-01-01T00:00:00Z')),
		       (SELECT n.id FROM notifications n WHERE n.channel = ? ORDER BY n.created_at DESC, n.id DESC LIMIT 1)
		WHERE (SELECT COUNT(*) FROM notifications WHERE channel = ?) > 0
	`, channel, channel, channel, channel)
}

// scanChannelSummaryRow 扫描 channel_summary JOIN notifications 的一行
func scanChannelSummaryRow(s rowScanner) (*notification.ChannelStats, error) {
	var (
		cs                                                     notification.ChannelStats
		id, tag, channel, title, body, priorityStr             string
		readAtStr, dismissedAtStr, notAfterStr, notBeforeStr    string
		createdAtStr, updatedAtStr, detailURLStr                string
	)
	err := s.Scan(
		&cs.Channel, &cs.UnreadCount,
		&id, &tag, &channel, &title, &body, &priorityStr,
		&readAtStr, &dismissedAtStr, &notAfterStr, &notBeforeStr,
		&createdAtStr, &updatedAtStr, &detailURLStr,
	)
	if err == sql.ErrNoRows {
		return nil, err
	} else if err != nil {
		return nil, err
	}

	// LEFT JOIN 无匹配时 id 为空字符串
	if id == "" {
		return &cs, nil
	}

	notif, err := parseNotificationRow(id, tag, channel, title, body, priorityStr,
		readAtStr, dismissedAtStr, notAfterStr, notBeforeStr,
		createdAtStr, updatedAtStr, detailURLStr)
	if err != nil {
		return nil, err
	}
	cs.LatestNotification = notif
	return &cs, nil
}

// #region 辅助方法

type rowScanner interface {
	Scan(dest ...any) error
}

// parseNotificationRow 从已扫描的字符串字段构造领域对象
func parseNotificationRow(
	id, tag, channel, title, body, priorityStr string,
	readAtStr, dismissedAtStr, notAfterStr, notBeforeStr string,
	createdAtStr, updatedAtStr, detailURLStr string,
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
	detailURL, err := scalar.ParseURI(detailURLStr)
	if err != nil {
		return nil, err
	}

	return notification.FromRepository(
		scalar.ToID(id), tag, channel, title, body, priority,
		readAt, dismissedAt, notAfter, notBefore,
		createdAt, updatedAt, detailURL,
	), nil
}

// scanNotificationRow 从 sql.Row 或 sql.Rows 扫描一行并构造领域对象
func scanNotificationRow(s rowScanner) (*notification.Notification, error) {
	var id, tag, channel, title, body, priorityStr, readAtStr, dismissedAtStr, notAfterStr, notBeforeStr, createdAtStr, updatedAtStr, detailURLStr string
	err := s.Scan(
		&id, &tag, &channel, &title, &body, &priorityStr, &readAtStr, &dismissedAtStr, &notAfterStr, &notBeforeStr, &createdAtStr, &updatedAtStr, &detailURLStr,
	)
	if err != nil {
		return nil, err
	}

	return parseNotificationRow(id, tag, channel, title, body, priorityStr,
		readAtStr, dismissedAtStr, notAfterStr, notBeforeStr,
		createdAtStr, updatedAtStr, detailURLStr)
}

func parseTime(s string) (time.Time, error) {
	if s == "" || s == "0001-01-01T00:00:00Z" {
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
		return "0001-01-01T00:00:00Z"
	}
	return t.Format(time.RFC3339Nano)
}

// #endregion
