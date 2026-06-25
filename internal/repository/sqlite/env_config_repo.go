package sqlite

import (
	"context"
	"database/sql"

	"easyserver/internal/model"
	"easyserver/internal/repository"
)

// EnvConfigRepository implements repository.EnvConfigRepository for SQLite
type EnvConfigRepository struct {
	db *sql.DB
}

// NewEnvConfigRepository creates a new EnvConfigRepository
func NewEnvConfigRepository(db *sql.DB) repository.EnvConfigRepository {
	return &EnvConfigRepository{db: db}
}

// ListEnvConfigs returns all environment configurations for a runtime
func (r *EnvConfigRepository) ListEnvConfigs(ctx context.Context, runtimeID int64) ([]model.EnvConfig, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, name, value, runtime_id, is_global, created_at, updated_at FROM env_configs WHERE runtime_id = ? ORDER BY name",
		runtimeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []model.EnvConfig
	for rows.Next() {
		var c model.EnvConfig
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

// GetEnvConfig returns a specific environment configuration
func (r *EnvConfigRepository) GetEnvConfig(ctx context.Context, id int64) (*model.EnvConfig, error) {
	c := &model.EnvConfig{}
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

// CreateEnvConfig creates a new environment configuration
func (r *EnvConfigRepository) CreateEnvConfig(ctx context.Context, c *model.EnvConfig) error {
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

// UpdateEnvConfig updates an environment configuration
func (r *EnvConfigRepository) UpdateEnvConfig(ctx context.Context, c *model.EnvConfig) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE env_configs SET name = ?, value = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		c.Name, c.Value, c.ID,
	)
	return err
}

// DeleteEnvConfig deletes an environment configuration
func (r *EnvConfigRepository) DeleteEnvConfig(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM env_configs WHERE id = ?", id)
	return err
}

// ListPathEntries returns all PATH entries for a runtime
func (r *EnvConfigRepository) ListPathEntries(ctx context.Context, runtimeID int64) ([]model.PathEntry, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, path, runtime_id, is_global, order_num, created_at FROM path_entries WHERE runtime_id = ? ORDER BY order_num",
		runtimeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []model.PathEntry
	for rows.Next() {
		var e model.PathEntry
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

// CreatePathEntry creates a new PATH entry
func (r *EnvConfigRepository) CreatePathEntry(ctx context.Context, e *model.PathEntry) error {
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

// DeletePathEntry deletes a PATH entry
func (r *EnvConfigRepository) DeletePathEntry(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM path_entries WHERE id = ?", id)
	return err
}

// ReorderPathEntries reorders PATH entries
func (r *EnvConfigRepository) ReorderPathEntries(ctx context.Context, runtimeID int64, ids []int64) error {
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

// ListGlobalConfigs returns all global configurations, optionally filtered by category
func (r *EnvConfigRepository) ListGlobalConfigs(ctx context.Context, category string) ([]model.GlobalConfig, error) {
	var rows *sql.Rows
	var err error

	if category != "" {
		rows, err = r.db.QueryContext(ctx,
			"SELECT id, category, key, value, description, created_at, updated_at FROM global_configs WHERE category = ? ORDER BY category, key",
			category,
		)
	} else {
		rows, err = r.db.QueryContext(ctx,
			"SELECT id, category, key, value, description, created_at, updated_at FROM global_configs ORDER BY category, key",
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []model.GlobalConfig
	for rows.Next() {
		var c model.GlobalConfig
		err := rows.Scan(&c.ID, &c.Category, &c.Key, &c.Value, &c.Description, &c.CreatedAt, &c.UpdatedAt)
		if err != nil {
			continue
		}
		configs = append(configs, c)
	}

	return configs, nil
}

// GetGlobalConfig returns a specific global configuration
func (r *EnvConfigRepository) GetGlobalConfig(ctx context.Context, id int64) (*model.GlobalConfig, error) {
	c := &model.GlobalConfig{}
	err := r.db.QueryRowContext(ctx,
		"SELECT id, category, key, value, description, created_at, updated_at FROM global_configs WHERE id = ?",
		id,
	).Scan(&c.ID, &c.Category, &c.Key, &c.Value, &c.Description, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

// CreateGlobalConfig creates a new global configuration
func (r *EnvConfigRepository) CreateGlobalConfig(ctx context.Context, c *model.GlobalConfig) error {
	result, err := r.db.ExecContext(ctx,
		"INSERT INTO global_configs (category, key, value, description) VALUES (?, ?, ?, ?)",
		c.Category, c.Key, c.Value, c.Description,
	)
	if err != nil {
		return err
	}
	c.ID, _ = result.LastInsertId()
	return nil
}

// UpdateGlobalConfig updates a global configuration
func (r *EnvConfigRepository) UpdateGlobalConfig(ctx context.Context, c *model.GlobalConfig) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE global_configs SET value = ?, description = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		c.Value, c.Description, c.ID,
	)
	return err
}

// DeleteGlobalConfig deletes a global configuration
func (r *EnvConfigRepository) DeleteGlobalConfig(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM global_configs WHERE id = ?", id)
	return err
}

// CreateGlobalConfigIfNotExists creates a global config only if one with the same category+key doesn't exist
func (r *EnvConfigRepository) CreateGlobalConfigIfNotExists(ctx context.Context, c *model.GlobalConfig) error {
	var count int
	r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM global_configs WHERE category = ? AND key = ?", c.Category, c.Key).Scan(&count)
	if count == 0 {
		result, err := r.db.ExecContext(ctx,
			"INSERT INTO global_configs (category, key, value, description) VALUES (?, ?, ?, ?)",
			c.Category, c.Key, c.Value, c.Description,
		)
		if err != nil {
			return err
		}
		c.ID, _ = result.LastInsertId()
	}
	return nil
}
