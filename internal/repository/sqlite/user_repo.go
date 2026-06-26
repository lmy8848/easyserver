package sqlite

import (
	"database/sql"

	"easyserver/internal/auth"
	"easyserver/internal/repository"
)

// UserRepository migrated to auth package; forwarding constructor.
func NewUserRepository(db *sql.DB) repository.UserRepository {
	return auth.NewSQLiteUserRepository(db)
}
