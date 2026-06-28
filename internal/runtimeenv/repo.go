package runtimeenv

import (
	"context"

	"easyserver/internal/envconfig"
)

// Repository defines the interface for runtime environment data access
type Repository interface {
	// Query
	ListAll(ctx context.Context) ([]RuntimeEnvironment, error)
	ListByName(ctx context.Context, name string) ([]RuntimeEnvironment, error)
	GetDefault(ctx context.Context, name string) (*RuntimeEnvironment, error)
	GetByID(ctx context.Context, id int64) (*RuntimeEnvironment, error)
	GetByNameAndVersion(ctx context.Context, name, version string) (*RuntimeEnvironment, error)
	GetProgress(ctx context.Context, id int64) (progress int, step, logs, errorMessage string, err error)

	// Existence checks
	ExistsByNameAndVersion(ctx context.Context, name, version string) (bool, error)
	ExistsSimilarVersion(ctx context.Context, name, majorVersion string) (bool, error)
	HasDefault(ctx context.Context, name string) (bool, error)

	// Create/Delete
	Create(ctx context.Context, name, version, path, status string) (int64, error)
	Delete(ctx context.Context, id int64) error

	// Status & progress updates
	UpdateProgress(ctx context.Context, id int64, progress int, step, logs string) error
	UpdateStatus(ctx context.Context, id int64, status string) error
	UpdateStatusToFailed(ctx context.Context, id int64, errorMessage string) error
	UpdateStatusToUninstallFailed(ctx context.Context, id int64, errorMessage string) error
	UpdateStatusToInstalled(ctx context.Context, id int64, path string) error

	// Default management
	ResetDefaults(ctx context.Context, name string) error
	SetDefaultByID(ctx context.Context, id int64) error
	SetDefaultByNameAndVersion(ctx context.Context, name, version string) error

	// Cleanup related data
	CleanupEnvConfigs(ctx context.Context, runtimeID int64) (int64, error)
	CleanupPathEntries(ctx context.Context, runtimeID int64) (int64, error)

	// Related resource queries
	ListEnvConfigsByRuntimeID(ctx context.Context, runtimeID int64) ([]envconfig.EnvConfig, error)
	ListPathEntriesByRuntimeID(ctx context.Context, runtimeID int64) ([]envconfig.PathEntry, error)

	// Runtime version cache
	InitRuntimeVersionsTable(ctx context.Context) error
	ListRuntimeVersions(ctx context.Context, name string) ([]RuntimeVersion, error)
	UpsertRuntimeVersion(ctx context.Context, name, version string, lts bool, stable bool) error

	// Mirrors
	CountMirrors(ctx context.Context) (int, error)
	SeedMirrors(ctx context.Context, mirrors []RuntimeMirror) error
	ListMirrors(ctx context.Context) ([]RuntimeMirror, error)
	GetMirror(ctx context.Context, id int64) (*RuntimeMirror, error)
	UpdateMirror(ctx context.Context, id int64, envValue string, enabled int) error
	CreateMirror(ctx context.Context, mirror *RuntimeMirror) (int64, error)
	DisableOtherMirrors(ctx context.Context, envKey string, excludeID int64) error
	DeleteMirror(ctx context.Context, id int64) error
}
