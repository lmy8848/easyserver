package dbserver

import (
	"context"
)

// Repository defines the interface for database server data access
type Repository interface {
	// Server operations
	ListServers(ctx context.Context) ([]DBServer, error)
	GetServer(ctx context.Context, id int64) (*DBServer, error)
	SeedServer(ctx context.Context, name, displayName, description string, defaultPort int) error

	// Version operations
	ListVersions(ctx context.Context, dbServerID int64) ([]DBInstance, error)
	GetVersion(ctx context.Context, id int64) (*DBInstance, error)
	CountVersionsByServerAndVersion(ctx context.Context, dbServerID int64, version string) (int, error)
	CreateVersion(ctx context.Context, dbServerID int64, version, serviceName string, port int, status string) (int64, error)
	CreateContainerVersion(ctx context.Context, v *DBInstance) (int64, error)
	DeleteVersion(ctx context.Context, id int64) error
	CountDatabasesByVersion(ctx context.Context, versionID int64) (int, error)

	// Status updates
	UpdateVersionStatus(ctx context.Context, id int64, status string) error
	UpdateVersionPort(ctx context.Context, id int64, port int) error
	UpdateVersionPassword(ctx context.Context, id int64, encryptedPassword string) error
	UpdateServerStatus(ctx context.Context, id int64, status, versionSummary string) error
}
