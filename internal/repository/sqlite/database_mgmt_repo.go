package sqlite

import (
	"database/sql"

	"easyserver/internal/database_mgmt"
)

// NewDatabaseMgmtRepository creates a new database management repository.
// Delegates to the domain package implementation.
func NewDatabaseMgmtRepository(db *sql.DB) database_mgmt.Repository {
	return database_mgmt.NewSQLiteRepository(db)
}
