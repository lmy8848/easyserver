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

func (r *sqliteRepo) ListEnvConfigs(ctx context.Context) ([]EnvConfig, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, name, value, enabled, created_at, updated_at FROM env_configs ORDER BY name",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []EnvConfig
	for rows.Next() {
		var c EnvConfig
		err := rows.Scan(&c.ID, &c.Name, &c.Value, &c.Enabled, &c.CreatedAt, &c.UpdatedAt)
		if err != nil {
			return nil, err
		}
		configs = append(configs, c)
	}

	return configs, nil
}

func (r *sqliteRepo) GetEnvConfig(ctx context.Context, id int64) (*EnvConfig, error) {
	c := &EnvConfig{}
	err := r.db.QueryRowContext(ctx,
		"SELECT id, name, value, enabled, created_at, updated_at FROM env_configs WHERE id = ?",
		id,
	).Scan(&c.ID, &c.Name, &c.Value, &c.Enabled, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *sqliteRepo) CreateEnvConfig(ctx context.Context, c *EnvConfig) error {
	result, err := r.db.ExecContext(ctx,
		"INSERT INTO env_configs (name, value, enabled) VALUES (?, ?, ?)",
		c.Name, c.Value, c.Enabled,
	)
	if err != nil {
		return err
	}
	c.ID, _ = result.LastInsertId()
	return nil
}

func (r *sqliteRepo) UpdateEnvConfig(ctx context.Context, c *EnvConfig) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE env_configs SET name = ?, value = ?, enabled = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		c.Name, c.Value, c.Enabled, c.ID,
	)
	return err
}

func (r *sqliteRepo) DeleteEnvConfig(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM env_configs WHERE id = ?", id)
	return err
}

func (r *sqliteRepo) ListPathEntries(ctx context.Context) ([]PathEntry, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, path, enabled, order_num, created_at FROM path_entries ORDER BY order_num",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []PathEntry
	for rows.Next() {
		var e PathEntry
		err := rows.Scan(&e.ID, &e.Path, &e.Enabled, &e.Order, &e.CreatedAt)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}

	return entries, nil
}

func (r *sqliteRepo) CreatePathEntry(ctx context.Context, e *PathEntry) error {
	// Get max order
	var maxOrder int
	r.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(order_num), 0) FROM path_entries").Scan(&maxOrder)

	result, err := r.db.ExecContext(ctx,
		"INSERT INTO path_entries (path, enabled, order_num) VALUES (?, ?, ?)",
		e.Path, e.Enabled, maxOrder+1,
	)
	if err != nil {
		return err
	}
	e.ID, _ = result.LastInsertId()
	e.Order = maxOrder + 1
	return nil
}

func (r *sqliteRepo) UpdatePathEntry(ctx context.Context, e *PathEntry) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE path_entries SET path = ?, enabled = ?, order_num = ? WHERE id = ?",
		e.Path, e.Enabled, e.Order, e.ID,
	)
	return err
}

func (r *sqliteRepo) DeletePathEntry(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM path_entries WHERE id = ?", id)
	return err
}

func (r *sqliteRepo) ReorderPathEntries(ctx context.Context, ids []int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for i, id := range ids {
		_, err := tx.ExecContext(ctx,
			"UPDATE path_entries SET order_num = ? WHERE id = ?",
			i+1, id,
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}
