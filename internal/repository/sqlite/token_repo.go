package sqlite

import (
	"database/sql"

	"easyserver/internal/auth"
	"easyserver/internal/repository"
)

// TokenBlacklistRepository migrated to auth package; forwarding constructor.
func NewTokenBlacklistRepository(db *sql.DB) repository.TokenBlacklistRepository {
	return auth.NewSQLiteTokenRepository(db)
}
