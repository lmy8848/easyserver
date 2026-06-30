package runtimeenv

import (
	"context"
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
	Create(ctx context.Context, lang, version, exact, status string) (int64, error)
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
	ListDefaults(ctx context.Context) ([]GlobalDefaultEntry, error)

	// Cleanup related data
	CleanupGlobalDefaultsByRuntimeID(ctx context.Context, runtimeID int64) (int64, error)

	GetConflictingReferences(ctx context.Context, runtimeID int64) ([]string, error)
	HealState(ctx context.Context) error
}
