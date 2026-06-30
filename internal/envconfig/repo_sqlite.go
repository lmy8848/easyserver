package envconfig

import (
	"context"
	"database/sql"
)

// sqliteRepo implements Repository for SQLite
type sqliteRepo struct {
	db *sql.DB
}

// NewSQLiteRepository creates a new SQLite-backed Repository
func NewSQLiteRepository(db *sql.DB) Repository {
	return &sqliteRepo{db: db}
}

func (r *sqliteRepo) ListEnvConfigs(ctx context.Context, runtimeID int64) ([]EnvConfig, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, name, value, runtime_id, is_global, created_at, updated_at FROM env_configs WHERE runtime_id = ? ORDER BY name",
		runtimeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []EnvConfig
	for rows.Next() {
		var c EnvConfig
		var isGlobal int
		err := rows.Scan(&c.ID, &c.Name, &c.Value, &c.RuntimeID, &isGlobal, &c.CreatedAt, &c.UpdatedAt)
		if err != nil {
			continue
		}
		c.IsGlobal = isGlobal != 0
		configs = append(configs, c)
	}

	return configs, nil
}

func (r *sqliteRepo) GetEnvConfig(ctx context.Context, id int64) (*EnvConfig, error) {
	c := &EnvConfig{}
	var isGlobal int
	err := r.db.QueryRowContext(ctx,
		"SELECT id, name, value, runtime_id, is_global, created_at, updated_at FROM env_configs WHERE id = ?",
		id,
	).Scan(&c.ID, &c.Name, &c.Value, &c.RuntimeID, &isGlobal, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.IsGlobal = isGlobal != 0
	return c, nil
}

func (r *sqliteRepo) CreateEnvConfig(ctx context.Context, c *EnvConfig) error {
	result, err := r.db.ExecContext(ctx,
		"INSERT INTO env_configs (name, value, runtime_id, is_global) VALUES (?, ?, ?, ?)",
		c.Name, c.Value, c.RuntimeID, c.IsGlobal,
	)
	if err != nil {
		return err
	}
	c.ID, _ = result.LastInsertId()
	return nil
}

func (r *sqliteRepo) UpdateEnvConfig(ctx context.Context, c *EnvConfig) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE env_configs SET name = ?, value = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		c.Name, c.Value, c.ID,
	)
	return err
}

func (r *sqliteRepo) DeleteEnvConfig(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM env_configs WHERE id = ?", id)
	return err
}

func (r *sqliteRepo) ListPathEntries(ctx context.Context, runtimeID int64) ([]PathEntry, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, path, runtime_id, is_global, order_num, created_at FROM path_entries WHERE runtime_id = ? ORDER BY order_num",
		runtimeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []PathEntry
	for rows.Next() {
		var e PathEntry
		var isGlobal int
		err := rows.Scan(&e.ID, &e.Path, &e.RuntimeID, &isGlobal, &e.Order, &e.CreatedAt)
		if err != nil {
			continue
		}
		e.IsGlobal = isGlobal != 0
		entries = append(entries, e)
	}

	return entries, nil
}

func (r *sqliteRepo) CreatePathEntry(ctx context.Context, e *PathEntry) error {
	// Get max order
	var maxOrder int
	r.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(order_num), 0) FROM path_entries WHERE runtime_id = ?", e.RuntimeID).Scan(&maxOrder)

	result, err := r.db.ExecContext(ctx,
		"INSERT INTO path_entries (path, runtime_id, is_global, order_num) VALUES (?, ?, ?, ?)",
		e.Path, e.RuntimeID, e.IsGlobal, maxOrder+1,
	)
	if err != nil {
		return err
	}
	e.ID, _ = result.LastInsertId()
	e.Order = maxOrder + 1
	return nil
}

func (r *sqliteRepo) DeletePathEntry(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM path_entries WHERE id = ?", id)
	return err
}

func (r *sqliteRepo) ReorderPathEntries(ctx context.Context, runtimeID int64, ids []int64) error {
	for i, id := range ids {
		_, err := r.db.ExecContext(ctx,
			"UPDATE path_entries SET order_num = ? WHERE id = ? AND runtime_id = ?",
			i+1, id, runtimeID,
		)
		if err != nil {
			return err
		}
	}
	return nil
}
