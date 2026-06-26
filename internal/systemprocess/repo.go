package systemprocess

import "context"

// Repository defines the interface for service whitelist data access.
type Repository interface {
	Init(ctx context.Context) error
	List(ctx context.Context) ([]ServiceWhitelistEntry, error)
	Add(ctx context.Context, name string) error
	Delete(ctx context.Context, name string) error
}
