package sqlite

import (
	"context"
	"database/sql"

	"easyserver/internal/model"
	"easyserver/internal/repository"
)

// PackageRepository implements repository.PackageRepository for SQLite
type PackageRepository struct {
	db *sql.DB
}

// NewPackageRepository creates a new PackageRepository
func NewPackageRepository(db *sql.DB) repository.PackageRepository {
	return &PackageRepository{db: db}
}

// List returns all packages for a runtime, joined with runtime name
func (r *PackageRepository) List(ctx context.Context, runtimeID int64) ([]model.Package, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT p.id, p.runtime_id, r.name, p.name, p.version, p.scope, p.source, p.installed_at
		FROM packages p
		LEFT JOIN runtime_environments r ON p.runtime_id = r.id
		WHERE p.runtime_id = ?
		ORDER BY p.name
	`, runtimeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var packages []model.Package
	for rows.Next() {
		var pkg model.Package
		if err := rows.Scan(&pkg.ID, &pkg.RuntimeID, &pkg.RuntimeName, &pkg.Name, &pkg.Version, &pkg.Scope, &pkg.Source, &pkg.InstalledAt); err != nil {
			return nil, err
		}
		packages = append(packages, pkg)
	}
	return packages, rows.Err()
}

// Upsert inserts or replaces a package record
func (r *PackageRepository) Upsert(ctx context.Context, runtimeID int64, name, version, scope, source string) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT OR REPLACE INTO packages (runtime_id, name, version, scope, source) VALUES (?, ?, ?, ?, ?)",
		runtimeID, name, version, scope, source,
	)
	return err
}

// Delete removes a package by runtime_id, name, and scope
func (r *PackageRepository) Delete(ctx context.Context, runtimeID int64, name, scope string) error {
	_, err := r.db.ExecContext(ctx,
		"DELETE FROM packages WHERE runtime_id = ? AND name = ? AND scope = ?",
		runtimeID, name, scope,
	)
	return err
}
