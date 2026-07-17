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

	// 数据库 user_version 版本化管理，拒绝高版本操作
	var dbVersion int
	if err := db.QueryRow("PRAGMA user_version;").Scan(&dbVersion); err != nil {
		db.Close()
		return nil, fmt.Errorf("read database user_version: %w", err)
	}

	const expectedVersion = 1
	if dbVersion > expectedVersion {
		db.Close()
		return nil, fmt.Errorf("database schema version %d is newer than expected version %d", dbVersion, expectedVersion)
	}

	if dbVersion == 0 {
		// 初始化数据表及索引
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
			updated_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_notifications_channel_created ON notifications(channel, created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_notifications_tag ON notifications(tag);
		`
		if _, err := db.Exec(schema); err != nil {
			db.Close()
			return nil, fmt.Errorf("init notifications schema: %w", err)
		}

		// 写入 schema 版本
		if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d;", expectedVersion)); err != nil {
			db.Close()
			return nil, fmt.Errorf("write schema version: %w", err)
		}
	}

	return &NotificationRepository{db: db}, nil
}

// Close 释放数据库 resource
func (r *NotificationRepository) Close() error {
	return r.db.Close()
}

// Save 保存通知（新建或更新），返回是否为新建通知
// 若 notif.IsDeleted() 为 true，则执行物理删除
func (r *NotificationRepository) Save(ctx context.Context, notif *notification.Notification) (didCreate bool, err error) {
	if notif.IsDeleted() {
		// 物理删除
		_, err = r.db.ExecContext(ctx, "DELETE FROM notifications WHERE id = ?", notif.ID().String())
		return false, err
	}

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
	INSERT INTO notifications (id, tag, channel, title, body, priority, read_at, dismissed_at, not_after, not_before, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(tag) DO UPDATE SET
		title = excluded.title,
		body = excluded.body,
		priority = excluded.priority,
		read_at = excluded.read_at,
		dismissed_at = excluded.dismissed_at,
		not_after = excluded.not_after,
		not_before = excluded.not_before,
		updated_at = excluded.updated_at
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
	)
	if err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}

	return didCreate, nil
}

// Get 根据 ID 获取通知，不存在返回 nil, nil
func (r *NotificationRepository) Get(ctx context.Context, id string) (*notification.Notification, error) {
	query := `
	SELECT id, tag, channel, title, body, priority, read_at, dismissed_at, not_after, not_before, created_at, updated_at
	FROM notifications
	WHERE id = ?
	`
	var tag, channel, title, body, priorityStr, readAtStr, dismissedAtStr, notAfterStr, notBeforeStr, createdAtStr, updatedAtStr string
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&id, &tag, &channel, &title, &body, &priorityStr, &readAtStr, &dismissedAtStr, &notAfterStr, &notBeforeStr, &createdAtStr, &updatedAtStr,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	priority, err := enum.Parse[shared.NotificationPriorityMeta](priorityStr)
	if err != nil {
		return nil, fmt.Errorf("parse notification priority: %w", err)
	}

	return notification.FromRepository(
		scalar.ToID(id), tag, channel, title, body, priority,
		parseTime(readAtStr), parseTime(dismissedAtStr), parseTime(notAfterStr), parseTime(notBeforeStr),
		parseTime(createdAtStr), parseTime(updatedAtStr),
	), nil
}

// GetByTag 根据 tag 获取通知，不存在返回 nil, nil
func (r *NotificationRepository) GetByTag(ctx context.Context, tag string) (*notification.Notification, error) {
	query := `
	SELECT id, tag, channel, title, body, priority, read_at, dismissed_at, not_after, not_before, created_at, updated_at
	FROM notifications
	WHERE tag = ?
	`
	var id, channel, title, body, priorityStr, readAtStr, dismissedAtStr, notAfterStr, notBeforeStr, createdAtStr, updatedAtStr string
	err := r.db.QueryRowContext(ctx, query, tag).Scan(
		&id, &tag, &channel, &title, &body, &priorityStr, &readAtStr, &dismissedAtStr, &notAfterStr, &notBeforeStr, &createdAtStr, &updatedAtStr,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	priority, err := enum.Parse[shared.NotificationPriorityMeta](priorityStr)
	if err != nil {
		return nil, fmt.Errorf("parse notification priority: %w", err)
	}

	return notification.FromRepository(
		scalar.ToID(id), tag, channel, title, body, priority,
		parseTime(readAtStr), parseTime(dismissedAtStr), parseTime(notAfterStr), parseTime(notBeforeStr),
		parseTime(createdAtStr), parseTime(updatedAtStr),
	), nil
}

// Find 遍历所有通知，支持 options 粗筛（利用 Channel 加速过滤）
func (r *NotificationRepository) Find(ctx context.Context, options ...notification.FindOption) iter.Seq2[*notification.Notification, error] {
	opts := notification.NewFindOptions(options...)
	filter := opts.Filter()

	return func(yield func(*notification.Notification, error) bool) {
		query := `
		SELECT id, tag, channel, title, body, priority, read_at, dismissed_at, not_after, not_before, created_at, updated_at
		FROM notifications
		WHERE 1=1
		`
		args := []any{}
		if filter.Channel != nil {
			query += " AND channel = ?"
			args = append(args, *filter.Channel)
		}

		query += " ORDER BY created_at DESC, id DESC"

		rows, err := r.db.QueryContext(ctx, query, args...)
		if err != nil {
			yield(nil, err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var id, tag, channel, title, body, priorityStr, readAtStr, dismissedAtStr, notAfterStr, notBeforeStr, createdAtStr, updatedAtStr string
			err := rows.Scan(
				&id, &tag, &channel, &title, &body, &priorityStr, &readAtStr, &dismissedAtStr, &notAfterStr, &notBeforeStr, &createdAtStr, &updatedAtStr,
			)
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}

			priority, err := enum.Parse[shared.NotificationPriorityMeta](priorityStr)
			if err != nil {
				if !yield(nil, fmt.Errorf("parse priority: %w", err)) {
					return
				}
				continue
			}

			notif := notification.FromRepository(
				scalar.ToID(id), tag, channel, title, body, priority,
				parseTime(readAtStr), parseTime(dismissedAtStr), parseTime(notAfterStr), parseTime(notBeforeStr),
				parseTime(createdAtStr), parseTime(updatedAtStr),
			)

			if !yield(notif, nil) {
				return
			}
		}

		if err := rows.Err(); err != nil {
			yield(nil, err)
		}
	}
}

// Channels 遍历获取所有频道统计信息
func (r *NotificationRepository) Channels(ctx context.Context) iter.Seq2[*notification.ChannelStats, error] {
	return func(yield func(*notification.ChannelStats, error) bool) {
		nowStr := time.Now().Format(time.RFC3339Nano)

		query := `
		SELECT channel,
		       SUM(CASE WHEN (read_at IS NULL OR read_at = '0001-01-01T00:00:00Z' OR read_at = '') THEN 1 ELSE 0 END) as unread_count
		FROM notifications
		WHERE (not_after IS NULL OR not_after = '0001-01-01T00:00:00Z' OR not_after = '' OR not_after > ?)
		  AND (not_before IS NULL OR not_before = '0001-01-01T00:00:00Z' OR not_before = '' OR not_before <= ?)
		GROUP BY channel
		`
		rows, err := r.db.QueryContext(ctx, query, nowStr, nowStr)
		if err != nil {
			yield(nil, err)
			return
		}
		defer rows.Close()

		var statsList []*notification.ChannelStats
		for rows.Next() {
			var cs notification.ChannelStats
			if err := rows.Scan(&cs.Channel, &cs.UnreadCount); err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			statsList = append(statsList, &cs)
		}

		if err := rows.Err(); err != nil {
			yield(nil, err)
			return
		}
		rows.Close()

		for _, cs := range statsList {
			latestQuery := `
			SELECT id, tag, channel, title, body, priority, read_at, dismissed_at, not_after, not_before, created_at, updated_at
			FROM notifications
			WHERE channel = ?
			  AND (not_after IS NULL OR not_after = '0001-01-01T00:00:00Z' OR not_after = '' OR not_after > ?)
			  AND (not_before IS NULL OR not_before = '0001-01-01T00:00:00Z' OR not_before = '' OR not_before <= ?)
			ORDER BY created_at DESC, id DESC
			LIMIT 1
			`
			var id, tag, channel, title, body, priorityStr, readAtStr, dismissedAtStr, notAfterStr, notBeforeStr, createdAtStr, updatedAtStr string
			err := r.db.QueryRowContext(ctx, latestQuery, cs.Channel, nowStr, nowStr).Scan(
				&id, &tag, &channel, &title, &body, &priorityStr, &readAtStr, &dismissedAtStr, &notAfterStr, &notBeforeStr, &createdAtStr, &updatedAtStr,
			)
			if err == sql.ErrNoRows {
				cs.LatestNotification = nil
			} else if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			} else {
				priority, err := enum.Parse[shared.NotificationPriorityMeta](priorityStr)
				if err != nil {
					if !yield(nil, fmt.Errorf("parse latest priority: %w", err)) {
						return
					}
					continue
				}
				cs.LatestNotification = notification.FromRepository(
					scalar.ToID(id), tag, channel, title, body, priority,
					parseTime(readAtStr), parseTime(dismissedAtStr), parseTime(notAfterStr), parseTime(notBeforeStr),
					parseTime(createdAtStr), parseTime(updatedAtStr),
				)
			}

			if !yield(cs, nil) {
				return
			}
		}
	}
}

// 辅助方法：时间格式转换
func parseTime(s string) time.Time {
	if s == "" || s == "0001-01-01T00:00:00Z" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "0001-01-01T00:00:00Z"
	}
	return t.Format(time.RFC3339Nano)
}
