package notification

import (
	"context"
	"database/sql"

	"fmt"
)

// sqliteRepo implements Repository for SQLite
type sqliteRepo struct {
	db *sql.DB
}

// NewSQLiteRepository creates a new SQLite-backed notification Repository
func NewSQLiteRepository(db *sql.DB) Repository {
	return &sqliteRepo{db: db}
}

// List returns notifications with optional filters
func (r *sqliteRepo) List(ctx context.Context, unreadOnly bool, limit int) ([]Notification, error) {
	query := `SELECT id, type, title, message, level, is_read, COALESCE(metadata,''), created_at
	          FROM notifications`
	if unreadOnly {
		query += ` WHERE is_read = 0`
	}
	query += ` ORDER BY created_at DESC LIMIT ?`

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()

	var result []Notification
	for rows.Next() {
		var n Notification
		var isRead int
		if err := rows.Scan(&n.ID, &n.Type, &n.Title, &n.Message, &n.Level, &isRead, &n.Metadata, &n.CreatedAt); err != nil {
			continue
		}
		n.IsRead = isRead != 0
		result = append(result, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	return result, nil
}

// CountUnread returns the count of unread notifications
func (r *sqliteRepo) CountUnread(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM notifications WHERE is_read = 0").Scan(&count)
	return count, err
}

func (r *sqliteRepo) getByID(ctx context.Context, id int64) (*Notification, error) {
	var n Notification
	var isRead int
	err := r.db.QueryRowContext(ctx,
		"SELECT id, type, title, message, level, is_read, COALESCE(metadata,''), created_at FROM notifications WHERE id = ?",
		id,
	).Scan(&n.ID, &n.Type, &n.Title, &n.Message, &n.Level, &isRead, &n.Metadata, &n.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get notification by id: %w", err)
	}
	n.IsRead = isRead != 0
	return &n, nil
}

// Create adds a new notification
func (r *sqliteRepo) Create(ctx context.Context, req CreateNotificationRequest) (*Notification, error) {
	level := req.Level
	if level == "" {
		level = "info"
	}
	res, err := r.db.ExecContext(ctx,
		"INSERT INTO notifications (type, title, message, level, metadata) VALUES (?, ?, ?, ?, ?)",
		req.Type, req.Title, req.Message, level, req.Metadata,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return r.getByID(ctx, id)
}

// CreateIfNotExists adds a notification only if a similar one doesn't exist in the last hour
func (r *sqliteRepo) CreateIfNotExists(ctx context.Context, req CreateNotificationRequest) (*Notification, error) {
	level := req.Level
	if level == "" {
		level = "info"
	}

	var exists int
	_ = r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM notifications WHERE type = ? AND title = ? AND created_at > datetime('now', '-1 hour')",
		req.Type, req.Title,
	).Scan(&exists)

	if exists > 0 {
		return nil, nil
	}

	res, err := r.db.ExecContext(ctx,
		"INSERT INTO notifications (type, title, message, level, metadata) VALUES (?, ?, ?, ?, ?)",
		req.Type, req.Title, req.Message, level, req.Metadata,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return r.getByID(ctx, id)
}

// MarkAsRead marks a notification as read
func (r *sqliteRepo) MarkAsRead(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "UPDATE notifications SET is_read = 1 WHERE id = ?", id)
	return err
}

// MarkAllAsRead marks all notifications as read
func (r *sqliteRepo) MarkAllAsRead(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, "UPDATE notifications SET is_read = 1 WHERE is_read = 0")
	return err
}

// Delete removes a notification
func (r *sqliteRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM notifications WHERE id = ?", id)
	return err
}

// CleanOld removes notifications older than given days
func (r *sqliteRepo) CleanOld(ctx context.Context, days int) (int64, error) {
	result, err := r.db.ExecContext(ctx,
		"DELETE FROM notifications WHERE created_at < datetime('now', ?)",
		fmt.Sprintf("-%d days", days),
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
