package packagemanager

import "context"

// Repository defines the interface for package data access
type Repository interface {
	List(ctx context.Context, runtimeID int64) ([]Package, error)
	Upsert(ctx context.Context, runtimeID int64, name, version, scope, source string) error
	Delete(ctx context.Context, runtimeID int64, name, scope string) error
}
