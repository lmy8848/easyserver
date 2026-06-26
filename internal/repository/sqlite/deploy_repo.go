package sqlite

import (
	"database/sql"

	"easyserver/internal/deploy"
)

// NewDeployRepository creates a new deploy Repository backed by SQLite.
// This is a forwarding stub; the implementation lives in internal/deploy.
func NewDeployRepository(db *sql.DB) deploy.Repository {
	return deploy.NewSQLiteRepository(db)
}
