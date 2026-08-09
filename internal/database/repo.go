package database

import (
	"context"
)

// Repository defines data access for the whole database domain: container-backed
// instances and the backups produced from them. Logical databases/users are NOT
// persisted — they are queried live from the engine; the engine owns them.
type Repository interface {
	// Instance operations
	ListInstances(ctx context.Context, dbType DBType) ([]DBInstance, error)
	ListAllInstances(ctx context.Context) ([]DBInstance, error)
	GetInstance(ctx context.Context, id int64) (*DBInstance, error)
	CountInstancesByDBTypeAndVersion(ctx context.Context, dbType DBType, version string) (int, error)
	CreateInstance(ctx context.Context, v *DBInstance) (int64, error)
	DeleteInstance(ctx context.Context, id int64) error

	// Instance status updates
	UpdateInstanceStatus(ctx context.Context, id int64, status string) error
	UpdateInstancePort(ctx context.Context, id int64, port int) error
	UpdateInstancePassword(ctx context.Context, id int64, encryptedPassword string) error

	// Backup operations (scoped by instance + database name)
	CreateBackup(ctx context.Context, backup *DBBackup) (int64, error)
	UpdateBackupStatus(ctx context.Context, id int64, status string, fileSize int64, errorMessage string) error
	ListBackups(ctx context.Context, instanceID int64, databaseName string) ([]DBBackup, error)
	GetBackup(ctx context.Context, id int64) (*DBBackup, error)
	DeleteBackup(ctx context.Context, id int64) error
}
