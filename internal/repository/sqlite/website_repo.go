package sqlite

import (
	"database/sql"

	"easyserver/internal/repository"
	"easyserver/internal/web"
)

// NewWebsiteRepository is a forwarding stub; implementation moved to internal/web.
func NewWebsiteRepository(db *sql.DB) repository.WebsiteRepository {
	return web.NewSQLiteWebsiteRepository(db)
}
