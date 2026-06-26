package envconfig

import "context"

// Repository defines the interface for environment config data access
type Repository interface {
	// EnvConfig CRUD
	ListEnvConfigs(ctx context.Context, runtimeID int64) ([]EnvConfig, error)
	GetEnvConfig(ctx context.Context, id int64) (*EnvConfig, error)
	CreateEnvConfig(ctx context.Context, config *EnvConfig) error
	UpdateEnvConfig(ctx context.Context, config *EnvConfig) error
	DeleteEnvConfig(ctx context.Context, id int64) error

	// PathEntry CRUD
	ListPathEntries(ctx context.Context, runtimeID int64) ([]PathEntry, error)
	CreatePathEntry(ctx context.Context, entry *PathEntry) error
	DeletePathEntry(ctx context.Context, id int64) error
	ReorderPathEntries(ctx context.Context, runtimeID int64, ids []int64) error

	// GlobalConfig CRUD
	ListGlobalConfigs(ctx context.Context, category string) ([]GlobalConfig, error)
	GetGlobalConfig(ctx context.Context, id int64) (*GlobalConfig, error)
	CreateGlobalConfig(ctx context.Context, config *GlobalConfig) error
	UpdateGlobalConfig(ctx context.Context, config *GlobalConfig) error
	DeleteGlobalConfig(ctx context.Context, id int64) error
	CreateGlobalConfigIfNotExists(ctx context.Context, config *GlobalConfig) error
}
