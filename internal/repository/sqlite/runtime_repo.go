package sqlite

import (
	"database/sql"

	"easyserver/internal/runtimeenv"
)

// NewRuntimeRepository creates a new runtime repository backed by SQLite.
// This is a forwarding stub; the real implementation lives in internal/runtimeenv.
func NewRuntimeRepository(db *sql.DB) runtimeenv.Repository {
	return runtimeenv.NewSQLiteRepository(db)
}
