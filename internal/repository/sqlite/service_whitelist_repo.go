package sqlite

import (
	"database/sql"

	"easyserver/internal/systemprocess"
)

// NewServiceWhitelistRepository creates a new service whitelist repository.
// Delegates to the domain package implementation.
func NewServiceWhitelistRepository(db *sql.DB) systemprocess.Repository {
	return systemprocess.NewSQLiteRepository(db)
}
