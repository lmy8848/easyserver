package sqlite

import (
	"database/sql"

	"easyserver/internal/packagemanager"
)

// NewPackageRepository returns a packagemanager.Repository backed by SQLite.
// Kept for backward compatibility with existing wiring in cmd/server/main.go.
func NewPackageRepository(db *sql.DB) packagemanager.Repository {
	return packagemanager.NewSQLiteRepository(db)
}
