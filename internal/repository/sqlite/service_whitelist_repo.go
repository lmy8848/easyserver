package sqlite

import (
	"context"
	"database/sql"

	"easyserver/internal/model"
	"easyserver/internal/repository"
)

// ServiceWhitelistRepository implements repository.ServiceWhitelistRepository for SQLite
type ServiceWhitelistRepository struct {
	db *sql.DB
}

// NewServiceWhitelistRepository creates a new ServiceWhitelistRepository
func NewServiceWhitelistRepository(db *sql.DB) repository.ServiceWhitelistRepository {
	return &ServiceWhitelistRepository{db: db}
}

// Init ensures the service_whitelist table exists
func (r *ServiceWhitelistRepository) Init(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS service_whitelist (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	return err
}

// List returns all whitelisted services
func (r *ServiceWhitelistRepository) List(ctx context.Context) ([]model.ServiceWhitelistEntry, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id, name, created_at FROM service_whitelist ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []model.ServiceWhitelistEntry
	for rows.Next() {
		var e model.ServiceWhitelistEntry
		if err := rows.Scan(&e.ID, &e.Name, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// Add inserts a service into the whitelist (no-op if already exists)
func (r *ServiceWhitelistRepository) Add(ctx context.Context, name string) error {
	_, err := r.db.ExecContext(ctx, "INSERT OR IGNORE INTO service_whitelist (name) VALUES (?)", name)
	return err
}

// Delete removes a service from the whitelist
func (r *ServiceWhitelistRepository) Delete(ctx context.Context, name string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM service_whitelist WHERE name = ?", name)
	return err
}
