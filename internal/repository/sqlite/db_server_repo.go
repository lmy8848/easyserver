package sqlite

import (
	"database/sql"

	"easyserver/internal/dbserver"
)

// NewDBServerRepository creates a new DBServerRepository.
// Deprecated: use dbserver.NewSQLiteRepository directly.
func NewDBServerRepository(db *sql.DB) dbserver.Repository {
	return dbserver.NewSQLiteRepository(db)
}
