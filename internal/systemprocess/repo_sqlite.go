package systemprocess

import (
	"context"
	"database/sql"
)

// SQLiteRepository implements Repository for SQLite.
type SQLiteRepository struct {
	db *sql.DB
}

// NewSQLiteRepository creates a new SQLiteRepository.
func NewSQLiteRepository(db *sql.DB) *SQLiteRepository {
	return &SQLiteRepository{db: db}
}

// Init ensures the service_whitelist table exists.
func (r *SQLiteRepository) Init(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS service_whitelist (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	return err
}

// List returns all whitelisted services.
func (r *SQLiteRepository) List(ctx context.Context) ([]ServiceWhitelistEntry, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id, name, created_at FROM service_whitelist ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []ServiceWhitelistEntry
	for rows.Next() {
		var e ServiceWhitelistEntry
		if err := rows.Scan(&e.ID, &e.Name, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// Add inserts a service into the whitelist (no-op if already exists).
func (r *SQLiteRepository) Add(ctx context.Context, name string) error {
	_, err := r.db.ExecContext(ctx, "INSERT OR IGNORE INTO service_whitelist (name) VALUES (?)", name)
	return err
}

// Delete removes a service from the whitelist.
func (r *SQLiteRepository) Delete(ctx context.Context, name string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM service_whitelist WHERE name = ?", name)
	return err
}
