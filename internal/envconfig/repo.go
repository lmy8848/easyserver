package envconfig

import "context"

// Repository defines the interface for environment config data access
type Repository interface {
	// EnvConfig CRUD
	ListEnvConfigs(ctx context.Context) ([]EnvConfig, error)
	GetEnvConfig(ctx context.Context, id int64) (*EnvConfig, error)
	CreateEnvConfig(ctx context.Context, config *EnvConfig) error
	UpdateEnvConfig(ctx context.Context, config *EnvConfig) error
	DeleteEnvConfig(ctx context.Context, id int64) error

	// PathEntry CRUD
	ListPathEntries(ctx context.Context) ([]PathEntry, error)
	CreatePathEntry(ctx context.Context, e *PathEntry) error
	UpdatePathEntry(ctx context.Context, e *PathEntry) error
	DeletePathEntry(ctx context.Context, id int64) error
	ReorderPathEntries(ctx context.Context, ids []int64) error
}
