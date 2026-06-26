package sqlite

import (
	"database/sql"

	"easyserver/internal/notification"
)

// NewNotificationRepository creates a new notification Repository backed by SQLite.
func NewNotificationRepository(db *sql.DB) notification.Repository {
	return notification.NewSQLiteRepository(db)
}
