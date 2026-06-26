package sqlite

import (
	"database/sql"

	"easyserver/internal/cron"
)

// NewCronRepository creates a new cron Repository backed by SQLite.
// Delegates to cron.NewSQLiteRepository.
func NewCronRepository(db *sql.DB) cron.Repository {
	return cron.NewSQLiteRepository(db)
}
