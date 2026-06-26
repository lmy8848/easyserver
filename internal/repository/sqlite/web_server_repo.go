package sqlite

import (
	"database/sql"

	"easyserver/internal/repository"
	"easyserver/internal/web"
)

// NewWebServerRepository is a forwarding stub; implementation moved to internal/web.
func NewWebServerRepository(db *sql.DB) repository.WebServerRepository {
	return web.NewSQLiteServerRepository(db)
}
