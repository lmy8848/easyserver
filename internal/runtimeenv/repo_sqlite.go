package runtimeenv

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"easyserver/internal/envconfig"
)

// sqliteRepo implements Repository for SQLite
type sqliteRepo struct {
	db *sql.DB
}

// NewSQLiteRepository creates a new SQLite-backed runtime repository
func NewSQLiteRepository(db *sql.DB) Repository {
	return &sqliteRepo{db: db}
}

// ListAll returns all runtime environments ordered by name and version
func (r *sqliteRepo) ListAll(ctx context.Context) ([]RuntimeEnvironment, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, name, version, path, is_default, status, progress, progress_step, logs, error_message, installed_at FROM runtime_environments ORDER BY name, version",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var environments []RuntimeEnvironment
	for rows.Next() {
		var env RuntimeEnvironment
		var isDefault int
		err := rows.Scan(&env.ID, &env.Name, &env.Version, &env.Path, &isDefault, &env.Status, &env.Progress, &env.ProgressStep, &env.Logs, &env.ErrorMessage, &env.InstalledAt)
		if err != nil {
			return nil, err
		}
		env.IsDefault = isDefault != 0
		environments = append(environments, env)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate: %w", err)
	}

	return environments, nil
}

// ListByName returns all versions of a specific runtime environment
func (r *sqliteRepo) ListByName(ctx context.Context, name string) ([]RuntimeEnvironment, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, name, version, path, is_default, status, progress, progress_step, logs, error_message, installed_at FROM runtime_environments WHERE name = ? ORDER BY version",
		name,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var environments []RuntimeEnvironment
	for rows.Next() {
		var env RuntimeEnvironment
		var isDefault int
		err := rows.Scan(&env.ID, &env.Name, &env.Version, &env.Path, &isDefault, &env.Status, &env.Progress, &env.ProgressStep, &env.Logs, &env.ErrorMessage, &env.InstalledAt)
		if err != nil {
			return nil, err
		}
		env.IsDefault = isDefault != 0
		environments = append(environments, env)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate: %w", err)
	}

	return environments, nil
}

// GetDefault returns the default version of a runtime environment
func (r *sqliteRepo) GetDefault(ctx context.Context, name string) (*RuntimeEnvironment, error) {
	env := &RuntimeEnvironment{}
	var isDefault int
	err := r.db.QueryRowContext(ctx,
		"SELECT id, name, version, path, is_default, status, installed_at FROM runtime_environments WHERE name = ? AND is_default = 1",
		name,
	).Scan(&env.ID, &env.Name, &env.Version, &env.Path, &isDefault, &env.Status, &env.InstalledAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	env.IsDefault = isDefault != 0
	return env, nil
}

// GetByID returns a runtime environment by ID
func (r *sqliteRepo) GetByID(ctx context.Context, id int64) (*RuntimeEnvironment, error) {
	env := &RuntimeEnvironment{}
	var isDefault int
	err := r.db.QueryRowContext(ctx,
		"SELECT id, name, version, path, is_default, status, progress, progress_step, logs, error_message, installed_at FROM runtime_environments WHERE id = ?",
		id,
	).Scan(&env.ID, &env.Name, &env.Version, &env.Path, &isDefault, &env.Status, &env.Progress, &env.ProgressStep, &env.Logs, &env.ErrorMessage, &env.InstalledAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	env.IsDefault = isDefault != 0
	return env, nil
}

// GetByNameAndVersion returns a specific runtime environment by name and version
func (r *sqliteRepo) GetByNameAndVersion(ctx context.Context, name, version string) (*RuntimeEnvironment, error) {
	env := &RuntimeEnvironment{}
	var isDefault int
	err := r.db.QueryRowContext(ctx,
		"SELECT id, name, version, path, is_default, status, installed_at FROM runtime_environments WHERE name = ? AND version = ?",
		name, version,
	).Scan(&env.ID, &env.Name, &env.Version, &env.Path, &isDefault, &env.Status, &env.InstalledAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	env.IsDefault = isDefault != 0
	return env, nil
}

// GetProgress returns the installation progress for a runtime environment
func (r *sqliteRepo) GetProgress(ctx context.Context, id int64) (int, string, string, string, error) {
	var progress int
	var step, logs, errorMessage string
	err := r.db.QueryRowContext(ctx,
		"SELECT progress, progress_step, logs, error_message FROM runtime_environments WHERE id = ?",
		id,
	).Scan(&progress, &step, &logs, &errorMessage)
	if err != nil {
		return 0, "", "", "", err
	}
	return progress, step, logs, errorMessage, nil
}

// ExistsByNameAndVersion checks if a runtime environment with the given name and version exists
func (r *sqliteRepo) ExistsByNameAndVersion(ctx context.Context, name, version string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM runtime_environments WHERE name = ? AND version = ?",
		name, version,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ExistsSimilarVersion checks if a similar version exists (e.g., "17" matches "17.0.19")
func (r *sqliteRepo) ExistsSimilarVersion(ctx context.Context, name, majorVersion string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM runtime_environments WHERE name = ? AND (version = ? OR version LIKE ?)",
		name, majorVersion, majorVersion+".%",
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// HasDefault checks if a runtime environment has any default version set
func (r *sqliteRepo) HasDefault(ctx context.Context, name string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM runtime_environments WHERE name = ? AND is_default = 1",
		name,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Create inserts a new runtime environment and returns its ID
func (r *sqliteRepo) Create(ctx context.Context, name, version, path, status string) (int64, error) {
	result, err := r.db.ExecContext(ctx,
		"INSERT INTO runtime_environments (name, version, path, status) VALUES (?, ?, ?, ?)",
		name, version, path, status,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// Delete removes a runtime environment by ID
func (r *sqliteRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM runtime_environments WHERE id = ?", id)
	return err
}

// UpdateProgress updates the installation progress fields
func (r *sqliteRepo) UpdateProgress(ctx context.Context, id int64, progress int, step, logs string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE runtime_environments SET progress = ?, progress_step = ?, logs = ? WHERE id = ?",
		progress, step, logs, id,
	)
	return err
}

// UpdateStatus updates the status of a runtime environment
func (r *sqliteRepo) UpdateStatus(ctx context.Context, id int64, status string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE runtime_environments SET status = ? WHERE id = ?",
		status, id,
	)
	return err
}

// UpdateStatusToFailed marks a runtime environment as failed with an error message
func (r *sqliteRepo) UpdateStatusToFailed(ctx context.Context, id int64, errorMessage string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE runtime_environments SET status = 'failed', error_message = ?, progress = 0, progress_step = 'failed' WHERE id = ?",
		errorMessage, id,
	)
	return err
}

// UpdateStatusToInstalled marks a runtime environment as installed with its path
func (r *sqliteRepo) UpdateStatusToInstalled(ctx context.Context, id int64, path string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE runtime_environments SET status = 'installed', path = ?, progress = 100, progress_step = 'done' WHERE id = ?",
		path, id,
	)
	return err
}

// ResetDefaults clears the default flag for all versions of a runtime
func (r *sqliteRepo) ResetDefaults(ctx context.Context, name string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE runtime_environments SET is_default = 0 WHERE name = ?",
		name,
	)
	return err
}

// SetDefaultByID sets a specific runtime environment as the default by ID
func (r *sqliteRepo) SetDefaultByID(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE runtime_environments SET is_default = 1 WHERE id = ?",
		id,
	)
	return err
}

// SetDefaultByNameAndVersion sets a specific version as the default
func (r *sqliteRepo) SetDefaultByNameAndVersion(ctx context.Context, name, version string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE runtime_environments SET is_default = 1 WHERE name = ? AND version = ?",
		name, version,
	)
	return err
}

// CleanupEnvConfigs deletes environment configs associated with a runtime
func (r *sqliteRepo) CleanupEnvConfigs(ctx context.Context, runtimeID int64) (int64, error) {
	result, err := r.db.ExecContext(ctx, "DELETE FROM env_configs WHERE runtime_id = ?", runtimeID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// CleanupPathEntries deletes PATH entries associated with a runtime
func (r *sqliteRepo) CleanupPathEntries(ctx context.Context, runtimeID int64) (int64, error) {
	result, err := r.db.ExecContext(ctx, "DELETE FROM path_entries WHERE runtime_id = ?", runtimeID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ListEnvConfigsByRuntimeID returns environment configs for a runtime
func (r *sqliteRepo) ListEnvConfigsByRuntimeID(ctx context.Context, runtimeID int64) ([]envconfig.EnvConfig, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, name, value, runtime_id, is_global, created_at, updated_at FROM env_configs WHERE runtime_id = ?",
		runtimeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []envconfig.EnvConfig
	for rows.Next() {
		var c envconfig.EnvConfig
		var isGlobal int
		err := rows.Scan(&c.ID, &c.Name, &c.Value, &c.RuntimeID, &isGlobal, &c.CreatedAt, &c.UpdatedAt)
		if err != nil {
			return nil, err
		}
		c.IsGlobal = isGlobal != 0
		configs = append(configs, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate: %w", err)
	}

	return configs, nil
}

// ListPathEntriesByRuntimeID returns PATH entries for a runtime
func (r *sqliteRepo) ListPathEntriesByRuntimeID(ctx context.Context, runtimeID int64) ([]envconfig.PathEntry, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, path, runtime_id, is_global, order_num, created_at FROM path_entries WHERE runtime_id = ?",
		runtimeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []envconfig.PathEntry
	for rows.Next() {
		var e envconfig.PathEntry
		var isGlobal int
		err := rows.Scan(&e.ID, &e.Path, &e.RuntimeID, &isGlobal, &e.Order, &e.CreatedAt)
		if err != nil {
			return nil, err
		}
		e.IsGlobal = isGlobal != 0
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate: %w", err)
	}

	return entries, nil
}

// InitRuntimeVersionsTable creates the runtime_versions table and index (deprecated, handled by migrations)
func (r *sqliteRepo) InitRuntimeVersionsTable(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS runtime_versions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			version TEXT NOT NULL,
			lts INTEGER DEFAULT 0,
			stable INTEGER DEFAULT 1,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(name, version)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_runtime_versions_name ON runtime_versions(name)`,
	}
	for _, q := range queries {
		if _, err := r.db.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

// ListRuntimeVersions returns all cached versions for a runtime name
func (r *sqliteRepo) ListRuntimeVersions(ctx context.Context, name string) ([]RuntimeVersion, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, name, version, lts, stable, updated_at FROM runtime_versions WHERE name = ? ORDER BY version DESC",
		name,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []RuntimeVersion
	for rows.Next() {
		var v RuntimeVersion
		var lts, stable int
		err := rows.Scan(&v.ID, &v.Name, &v.Version, &lts, &stable, &v.UpdatedAt)
		if err != nil {
			return nil, err
		}
		v.LTS = lts != 0
		v.Stable = stable != 0
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate: %w", err)
	}
	return versions, nil
}

// UpsertRuntimeVersion inserts or replaces a cached runtime version
func (r *sqliteRepo) UpsertRuntimeVersion(ctx context.Context, name, version string, lts, stable bool) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO runtime_versions (name, version, lts, stable, updated_at) VALUES (?, ?, ?, ?, ?)`,
		name, version, lts, stable, time.Now(),
	)
	return err
}
