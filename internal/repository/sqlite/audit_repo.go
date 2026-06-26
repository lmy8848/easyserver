package sqlite

import (
	"database/sql"

	"easyserver/internal/audit"
)

// NewAuditRepository creates a new audit Repository backed by SQLite.
// This is a forwarding stub; the implementation lives in internal/audit.
func NewAuditRepository(db *sql.DB) audit.Repository {
	return audit.NewSQLiteRepository(db)
}
