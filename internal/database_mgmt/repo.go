package database_mgmt

import (
	"context"

	"easyserver/internal/dbserver"
)

// Repository defines the interface for database management data access.
type Repository interface {
	// Database operations
	ListDatabases(ctx context.Context, dbServerID int64) ([]Database, error)
	GetDatabase(ctx context.Context, dbServerID, id int64) (*Database, error)
	GetDatabaseByID(ctx context.Context, id int64) (*Database, error)
	CreateDatabase(ctx context.Context, dbServerID, dbVersionID int64, name, charset, description string) (int64, error)
	DeleteDatabase(ctx context.Context, dbServerID, id int64) error

	// DB User operations
	ListDBUsers(ctx context.Context, dbServerID int64) ([]DBUser, error)
	GetDBUser(ctx context.Context, dbServerID, id int64) (*DBUser, error)
	CreateDBUser(ctx context.Context, dbServerID int64, username, hashedPassword, host string) (int64, error)
	DeleteDBUser(ctx context.Context, dbServerID, id int64) error
	UpdateDBUserPrivileges(ctx context.Context, id int64, privileges string) error

	// Lookup helpers (lightweight queries)
	GetServer(ctx context.Context, id int64) (*dbserver.DBServer, error)
	GetVersion(ctx context.Context, id int64) (*dbserver.DBVersion, error)
	ListVersions(ctx context.Context, dbServerID int64) ([]dbserver.DBVersion, error)

	// Backup operations
	CreateBackup(ctx context.Context, backup *DBBackup) (int64, error)
	UpdateBackupStatus(ctx context.Context, id int64, status string, fileSize int64, errorMessage string) error
	ListBackups(ctx context.Context, databaseID int64) ([]DBBackup, error)
	GetBackup(ctx context.Context, id int64) (*DBBackup, error)
	DeleteBackup(ctx context.Context, id int64) error
}
