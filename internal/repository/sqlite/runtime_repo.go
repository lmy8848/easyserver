package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"easyserver/internal/model"
	"easyserver/internal/repository"
)

// RuntimeRepository implements repository.RuntimeRepository for SQLite
type RuntimeRepository struct {
	db *sql.DB
}

// NewRuntimeRepository creates a new RuntimeRepository
func NewRuntimeRepository(db *sql.DB) repository.RuntimeRepository {
	return &RuntimeRepository{db: db}
}

// ListAll returns all runtime environments ordered by name and version
func (r *RuntimeRepository) ListAll(ctx context.Context) ([]model.RuntimeEnvironment, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, name, version, path, is_default, status, progress, progress_step, logs, error_message, installed_at FROM runtime_environments ORDER BY name, version",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var environments []model.RuntimeEnvironment
	for rows.Next() {
		var env model.RuntimeEnvironment
		var isDefault int
		err := rows.Scan(&env.ID, &env.Name, &env.Version, &env.Path, &isDefault, &env.Status, &env.Progress, &env.ProgressStep, &env.Logs, &env.ErrorMessage, &env.InstalledAt)
		if err != nil {
			continue
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
func (r *RuntimeRepository) ListByName(ctx context.Context, name string) ([]model.RuntimeEnvironment, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, name, version, path, is_default, status, progress, progress_step, logs, error_message, installed_at FROM runtime_environments WHERE name = ? ORDER BY version",
		name,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var environments []model.RuntimeEnvironment
	for rows.Next() {
		var env model.RuntimeEnvironment
		var isDefault int
		err := rows.Scan(&env.ID, &env.Name, &env.Version, &env.Path, &isDefault, &env.Status, &env.Progress, &env.ProgressStep, &env.Logs, &env.ErrorMessage, &env.InstalledAt)
		if err != nil {
			continue
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
func (r *RuntimeRepository) GetDefault(ctx context.Context, name string) (*model.RuntimeEnvironment, error) {
	env := &model.RuntimeEnvironment{}
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
func (r *RuntimeRepository) GetByID(ctx context.Context, id int64) (*model.RuntimeEnvironment, error) {
	env := &model.RuntimeEnvironment{}
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
func (r *RuntimeRepository) GetByNameAndVersion(ctx context.Context, name, version string) (*model.RuntimeEnvironment, error) {
	env := &model.RuntimeEnvironment{}
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
func (r *RuntimeRepository) GetProgress(ctx context.Context, id int64) (int, string, string, string, error) {
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
func (r *RuntimeRepository) ExistsByNameAndVersion(ctx context.Context, name, version string) (bool, error) {
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
func (r *RuntimeRepository) ExistsSimilarVersion(ctx context.Context, name, majorVersion string) (bool, error) {
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
func (r *RuntimeRepository) HasDefault(ctx context.Context, name string) (bool, error) {
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
func (r *RuntimeRepository) Create(ctx context.Context, name, version, path, status string) (int64, error) {
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
func (r *RuntimeRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM runtime_environments WHERE id = ?", id)
	return err
}

// UpdateProgress updates the installation progress fields
func (r *RuntimeRepository) UpdateProgress(ctx context.Context, id int64, progress int, step, logs string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE runtime_environments SET progress = ?, progress_step = ?, logs = ? WHERE id = ?",
		progress, step, logs, id,
	)
	return err
}

// UpdateStatus updates the status of a runtime environment
func (r *RuntimeRepository) UpdateStatus(ctx context.Context, id int64, status string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE runtime_environments SET status = ? WHERE id = ?",
		status, id,
	)
	return err
}

// UpdateStatusToFailed marks a runtime environment as failed with an error message
func (r *RuntimeRepository) UpdateStatusToFailed(ctx context.Context, id int64, errorMessage string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE runtime_environments SET status = 'failed', error_message = ?, progress = 0, progress_step = 'failed' WHERE id = ?",
		errorMessage, id,
	)
	return err
}

// UpdateStatusToInstalled marks a runtime environment as installed with its path
func (r *RuntimeRepository) UpdateStatusToInstalled(ctx context.Context, id int64, path string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE runtime_environments SET status = 'installed', path = ?, progress = 100, progress_step = 'done' WHERE id = ?",
		path, id,
	)
	return err
}

// ResetDefaults clears the default flag for all versions of a runtime
func (r *RuntimeRepository) ResetDefaults(ctx context.Context, name string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE runtime_environments SET is_default = 0 WHERE name = ?",
		name,
	)
	return err
}

// SetDefaultByID sets a specific runtime environment as the default by ID
func (r *RuntimeRepository) SetDefaultByID(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE runtime_environments SET is_default = 1 WHERE id = ?",
		id,
	)
	return err
}

// SetDefaultByNameAndVersion sets a specific version as the default
func (r *RuntimeRepository) SetDefaultByNameAndVersion(ctx context.Context, name, version string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE runtime_environments SET is_default = 1 WHERE name = ? AND version = ?",
		name, version,
	)
	return err
}

// CleanupEnvConfigs deletes environment configs associated with a runtime
func (r *RuntimeRepository) CleanupEnvConfigs(ctx context.Context, runtimeID int64) (int64, error) {
	result, err := r.db.ExecContext(ctx, "DELETE FROM env_configs WHERE runtime_id = ?", runtimeID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// CleanupPathEntries deletes PATH entries associated with a runtime
func (r *RuntimeRepository) CleanupPathEntries(ctx context.Context, runtimeID int64) (int64, error) {
	result, err := r.db.ExecContext(ctx, "DELETE FROM path_entries WHERE runtime_id = ?", runtimeID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ListEnvConfigsByRuntimeID returns environment configs for a runtime
func (r *RuntimeRepository) ListEnvConfigsByRuntimeID(ctx context.Context, runtimeID int64) ([]model.EnvConfig, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, name, value, runtime_id, is_global, created_at, updated_at FROM env_configs WHERE runtime_id = ?",
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate: %w", err)
	}

	return configs, nil
}

// ListPathEntriesByRuntimeID returns PATH entries for a runtime
func (r *RuntimeRepository) ListPathEntriesByRuntimeID(ctx context.Context, runtimeID int64) ([]model.PathEntry, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, path, runtime_id, is_global, order_num, created_at FROM path_entries WHERE runtime_id = ?",
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate: %w", err)
	}

	return entries, nil
}
