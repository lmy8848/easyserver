package sqlite

import (
	"database/sql"

	"easyserver/internal/database_mgmt"
)

// NewDBBackupRepository creates a new database backup repository.
// Delegates to the domain package implementation.
func NewDBBackupRepository(db *sql.DB) database_mgmt.Repository {
	return database_mgmt.NewSQLiteRepository(db)
}
