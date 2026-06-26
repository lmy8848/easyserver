package sqlite

import (
	"database/sql"

	"easyserver/internal/envconfig"
)

// NewEnvConfigRepository creates a new envconfig.Repository backed by SQLite.
// Delegates to envconfig.NewSQLiteRepository.
func NewEnvConfigRepository(db *sql.DB) envconfig.Repository {
	return envconfig.NewSQLiteRepository(db)
}
