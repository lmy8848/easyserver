package sqlite

import (
	"database/sql"

	"easyserver/internal/process"
	"easyserver/internal/repository"
)

// NewProcessRepository creates a new process repository backed by SQLite.
// Delegates to the domain package implementation.
func NewProcessRepository(db *sql.DB) repository.ProcessRepository {
	return process.NewSQLiteRepository(db)
}
