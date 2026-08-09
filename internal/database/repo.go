package database

import (
	"context"
)

// Repository defines data access for the whole database domain: engine catalog,
// container-backed instances, and the logical databases/users/backups inside them.
type Repository interface {
	// Engine catalog operations
	ListServers(ctx context.Context) ([]DBServer, error)
	GetServer(ctx context.Context, id int64) (*DBServer, error)
	SeedServer(ctx context.Context, name, displayName, description string, defaultPort int) error

	// Instance operations
	ListVersions(ctx context.Context, dbServerID int64) ([]DBInstance, error)
	GetVersion(ctx context.Context, id int64) (*DBInstance, error)
	CountVersionsByServerAndVersion(ctx context.Context, dbServerID int64, version string) (int, error)
	CreateVersion(ctx context.Context, dbServerID int64, version, containerName string, port int, status string) (int64, error)
	CreateContainerVersion(ctx context.Context, v *DBInstance) (int64, error)
	DeleteVersion(ctx context.Context, id int64) error
	CountDatabasesByVersion(ctx context.Context, versionID int64) (int, error)

	// Instance status updates
	UpdateVersionStatus(ctx context.Context, id int64, status string) error
	UpdateVersionPort(ctx context.Context, id int64, port int) error
	UpdateVersionPassword(ctx context.Context, id int64, encryptedPassword string) error
	UpdateServerStatus(ctx context.Context, id int64, status, versionSummary string) error

	// Logical database operations
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

	// Backup operations
	CreateBackup(ctx context.Context, backup *DBBackup) (int64, error)
	UpdateBackupStatus(ctx context.Context, id int64, status string, fileSize int64, errorMessage string) error
	ListBackups(ctx context.Context, databaseID int64) ([]DBBackup, error)
	GetBackup(ctx context.Context, id int64) (*DBBackup, error)
	DeleteBackup(ctx context.Context, id int64) error
}
