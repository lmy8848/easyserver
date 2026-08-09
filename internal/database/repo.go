package database

import (
	"context"
)

// Repository defines data access for the whole database domain: container-backed
// instances, and the logical databases/users/backups inside them. The database
// engine is an enum (db_type), never a persisted catalog row.
type Repository interface {
	// Instance operations
	ListInstances(ctx context.Context, dbType DBType) ([]DBInstance, error)
	ListAllInstances(ctx context.Context) ([]DBInstance, error)
	GetInstance(ctx context.Context, id int64) (*DBInstance, error)
	CountInstancesByDBTypeAndVersion(ctx context.Context, dbType DBType, version string) (int, error)
	CreateInstance(ctx context.Context, v *DBInstance) (int64, error)
	DeleteInstance(ctx context.Context, id int64) error
	CountDatabasesByInstance(ctx context.Context, instanceID int64) (int, error)

	// Instance status updates
	UpdateInstanceStatus(ctx context.Context, id int64, status string) error
	UpdateInstancePort(ctx context.Context, id int64, port int) error
	UpdateInstancePassword(ctx context.Context, id int64, encryptedPassword string) error

	// Logical database operations (instance-scoped)
	ListDatabases(ctx context.Context, instanceID int64) ([]Database, error)
	GetDatabaseByID(ctx context.Context, id int64) (*Database, error)
	CreateDatabase(ctx context.Context, instanceID int64, name, charset, description string) (int64, error)
	DeleteDatabase(ctx context.Context, instanceID, id int64) error

	// DB User operations (instance-scoped)
	ListDBUsers(ctx context.Context, instanceID int64) ([]DBUser, error)
	GetDBUser(ctx context.Context, instanceID, id int64) (*DBUser, error)
	CreateDBUser(ctx context.Context, instanceID int64, username, hashedPassword, host string) (int64, error)
	DeleteDBUser(ctx context.Context, instanceID, id int64) error
	UpdateDBUserPrivileges(ctx context.Context, id int64, privileges string) error

	// Backup operations
	CreateBackup(ctx context.Context, backup *DBBackup) (int64, error)
	UpdateBackupStatus(ctx context.Context, id int64, status string, fileSize int64, errorMessage string) error
	ListBackups(ctx context.Context, databaseID int64) ([]DBBackup, error)
	GetBackup(ctx context.Context, id int64) (*DBBackup, error)
	DeleteBackup(ctx context.Context, id int64) error
}
