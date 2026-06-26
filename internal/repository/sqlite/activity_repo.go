package sqlite

import (
	"database/sql"

	"easyserver/internal/auth"
	"easyserver/internal/repository"
)

// ActivityRepository migrated to auth package; forwarding constructor.
func NewActivityRepository(db *sql.DB) repository.ActivityRepository {
	return auth.NewSQLiteActivityRepository(db)
}
