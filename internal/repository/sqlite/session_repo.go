package sqlite

import (
	"database/sql"

	"easyserver/internal/auth"
	"easyserver/internal/repository"
)

// SessionRepository migrated to auth package; forwarding constructor.
func NewSessionRepository(db *sql.DB) repository.SessionRepository {
	return auth.NewSQLiteSessionRepository(db)
}
