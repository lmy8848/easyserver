package sqlite

import (
	"database/sql"

	"easyserver/internal/monitor"
)

// NewMonitorRepository creates a new monitor Repository backed by SQLite.
func NewMonitorRepository(db *sql.DB) monitor.Repository {
	return monitor.NewSQLiteRepository(db)
}
