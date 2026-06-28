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
		"SELECT v.id, v.lang as name, v.exact as version, '' as path, CASE WHEN g.runtime_version_id IS NOT NULL THEN 1 ELSE 0 END as is_default, v.status, v.progress, v.progress_step, v.logs, v.error_message, v.installed_at FROM runtime_version v LEFT JOIN global_default g ON v.id = g.runtime_version_id ORDER BY v.lang, v.exact",
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
		"SELECT v.id, v.lang as name, v.exact as version, '' as path, CASE WHEN g.runtime_version_id IS NOT NULL THEN 1 ELSE 0 END as is_default, v.status, v.progress, v.progress_step, v.logs, v.error_message, v.installed_at FROM runtime_version v LEFT JOIN global_default g ON v.id = g.runtime_version_id WHERE v.lang = ? ORDER BY v.exact",
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
		"SELECT v.id, v.lang as name, v.exact as version, '' as path, 1 as is_default, v.status, v.installed_at FROM runtime_version v INNER JOIN global_default g ON v.id = g.runtime_version_id WHERE v.lang = ?",
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
		"SELECT v.id, v.lang as name, v.exact as version, '' as path, CASE WHEN g.runtime_version_id IS NOT NULL THEN 1 ELSE 0 END as is_default, v.status, v.progress, v.progress_step, v.logs, v.error_message, v.installed_at FROM runtime_version v LEFT JOIN global_default g ON v.id = g.runtime_version_id WHERE v.id = ?",
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
		"SELECT v.id, v.lang as name, v.exact as version, '' as path, CASE WHEN g.runtime_version_id IS NOT NULL THEN 1 ELSE 0 END as is_default, v.status, v.installed_at FROM runtime_version v LEFT JOIN global_default g ON v.id = g.runtime_version_id WHERE v.lang = ? AND v.exact = ?",
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
		"SELECT progress, progress_step, logs, error_message FROM runtime_version WHERE id = ?",
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
		"SELECT COUNT(*) FROM runtime_version WHERE lang = ? AND exact = ?",
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
		"SELECT COUNT(*) FROM runtime_version WHERE lang = ? AND (exact = ? OR exact LIKE ?)",
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
		"SELECT COUNT(*) FROM global_default WHERE lang = ?",
		name,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Create inserts a new runtime environment and returns its ID
func (r *sqliteRepo) Create(ctx context.Context, lang, version, exact, status string) (int64, error) {
	result, err := r.db.ExecContext(ctx,
		"INSERT INTO runtime_version (lang, version, exact, status) VALUES (?, ?, ?, ?)", lang, version, exact, status,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// Delete removes a runtime environment by ID
func (r *sqliteRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM runtime_version WHERE id = ?", id)
	return err
}

// UpdateProgress updates the installation progress fields
func (r *sqliteRepo) UpdateProgress(ctx context.Context, id int64, progress int, step, logs string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE runtime_version SET progress = ?, progress_step = ?, logs = ? WHERE id = ?",
		progress, step, logs, id,
	)
	return err
}

// UpdateStatus updates the status of a runtime environment
func (r *sqliteRepo) UpdateStatus(ctx context.Context, id int64, status string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE runtime_version SET status = ? WHERE id = ?",
		status, id,
	)
	return err
}

// UpdateStatusToFailed marks a runtime environment as failed with an error message
func (r *sqliteRepo) UpdateStatusToFailed(ctx context.Context, id int64, errorMessage string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE runtime_version SET status = 'failed', error_message = ?, progress = 0, progress_step = 'failed' WHERE id = ?",
		errorMessage, id,
	)
	return err
}

// UpdateStatusToUninstallFailed marks a runtime environment as failed during uninstall
func (r *sqliteRepo) UpdateStatusToUninstallFailed(ctx context.Context, id int64, errorMessage string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE runtime_version SET status = 'uninstall_failed', error_message = ?, progress = 0, progress_step = 'uninstall_failed' WHERE id = ?",
		errorMessage, id,
	)
	return err
}

// UpdateStatusToInstalled marks a runtime environment as installed with its path
func (r *sqliteRepo) UpdateStatusToInstalled(ctx context.Context, id int64, path string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE runtime_version SET status = 'installed', progress = 100, progress_step = 'done' WHERE id = ?",
		id,
	)
	return err
}

// ResetDefaults clears the default flag for all versions of a runtime
func (r *sqliteRepo) ResetDefaults(ctx context.Context, name string) error {
	_, err := r.db.ExecContext(ctx,
		"DELETE FROM global_default WHERE lang = ?",
		name,
	)
	return err
}

// SetDefaultByID sets a specific runtime environment as the default by ID
func (r *sqliteRepo) SetDefaultByID(ctx context.Context, id int64) error {
	var lang string
	err := r.db.QueryRowContext(ctx, "SELECT lang FROM runtime_version WHERE id = ?", id).Scan(&lang)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, "INSERT OR REPLACE INTO global_default (lang, runtime_version_id) VALUES (?, ?)", lang, id)
	return err
}

// SetDefaultByNameAndVersion sets a specific version as the default
func (r *sqliteRepo) SetDefaultByNameAndVersion(ctx context.Context, name, version string) error {
	var id int64
	err := r.db.QueryRowContext(ctx, "SELECT id FROM runtime_version WHERE lang = ? AND exact = ?", name, version).Scan(&id)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, "INSERT OR REPLACE INTO global_default (lang, runtime_version_id) VALUES (?, ?)", name, id)
	return err
}

// ListDefaults returns every (lang, exact) pair currently set as global default.
// Used by GenerateMiseConfig to render the [tools] section of /etc/mise/config.toml.
func (r *sqliteRepo) ListDefaults(ctx context.Context) ([]GlobalDefaultEntry, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT v.lang, v.exact FROM global_default g JOIN runtime_version v ON g.runtime_version_id = v.id ORDER BY v.lang",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GlobalDefaultEntry
	for rows.Next() {
		var e GlobalDefaultEntry
		if err := rows.Scan(&e.Lang, &e.Exact); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
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

// CleanupGlobalDefaultsByRuntimeID removes any global_default row that pins to
// a specific runtime_version row. Required before deleting that runtime_version
// because of the FK constraint, and ensures /etc/mise/config.toml stays in
// sync after Uninstall regenerates it.
func (r *sqliteRepo) CleanupGlobalDefaultsByRuntimeID(ctx context.Context, runtimeID int64) (int64, error) {
	result, err := r.db.ExecContext(ctx, "DELETE FROM global_default WHERE runtime_version_id = ?", runtimeID)
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

// CountMirrors returns the total number of mirrors
func (r *sqliteRepo) CountMirrors(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM runtime_mirror").Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// SeedMirrors inserts default mirrors
func (r *sqliteRepo) SeedMirrors(ctx context.Context, mirrors []RuntimeMirror) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, "INSERT INTO runtime_mirror (lang, env_key, env_value, enabled, source) VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, m := range mirrors {
		if _, err := stmt.ExecContext(ctx, m.Lang, m.EnvKey, m.EnvValue, m.Enabled, m.Source); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListMirrors returns all mirrors
func (r *sqliteRepo) ListMirrors(ctx context.Context) ([]RuntimeMirror, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id, lang, env_key, env_value, enabled, source, updated_at FROM runtime_mirror ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mirrors []RuntimeMirror
	for rows.Next() {
		var m RuntimeMirror
		if err := rows.Scan(&m.ID, &m.Lang, &m.EnvKey, &m.EnvValue, &m.Enabled, &m.Source, &m.UpdatedAt); err != nil {
			return nil, err
		}
		mirrors = append(mirrors, m)
	}
	return mirrors, rows.Err()
}

// GetMirror gets a mirror by id
func (r *sqliteRepo) GetMirror(ctx context.Context, id int64) (*RuntimeMirror, error) {
	var m RuntimeMirror
	err := r.db.QueryRowContext(ctx, "SELECT id, lang, env_key, env_value, enabled, source, updated_at FROM runtime_mirror WHERE id = ?", id).
		Scan(&m.ID, &m.Lang, &m.EnvKey, &m.EnvValue, &m.Enabled, &m.Source, &m.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// UpdateMirror updates env_value and enabled
func (r *sqliteRepo) UpdateMirror(ctx context.Context, id int64, envValue string, enabled int) error {
	_, err := r.db.ExecContext(ctx, "UPDATE runtime_mirror SET env_value = ?, enabled = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", envValue, enabled, id)
	return err
}

// CreateMirror creates a new user mirror
func (r *sqliteRepo) CreateMirror(ctx context.Context, mirror *RuntimeMirror) (int64, error) {
	res, err := r.db.ExecContext(ctx, "INSERT INTO runtime_mirror (lang, env_key, env_value, enabled, source) VALUES (?, ?, ?, ?, ?)",
		mirror.Lang, mirror.EnvKey, mirror.EnvValue, mirror.Enabled, mirror.Source)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// DeleteMirror deletes a mirror by id
func (r *sqliteRepo) DeleteMirror(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM runtime_mirror WHERE id = ?", id)
	return err
}

// DisableOtherMirrors disables all other mirrors with the same env key
func (r *sqliteRepo) DisableOtherMirrors(ctx context.Context, envKey string, excludeID int64) error {
	_, err := r.db.ExecContext(ctx, "UPDATE runtime_mirror SET enabled = 0, updated_at = CURRENT_TIMESTAMP WHERE env_key = ? AND id != ?", envKey, excludeID)
	return err
}

func (r *sqliteRepo) GetConflictingReferences(ctx context.Context, runtimeID int64) ([]string, error) {
	var conflicts []string
	rows, err := r.db.QueryContext(ctx, "SELECT name FROM processes WHERE runtime_version_id = ?", runtimeID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var pname string
			if err := rows.Scan(&pname); err == nil {
				conflicts = append(conflicts, "Process: "+pname)
			}
		}
	}

	rows, err = r.db.QueryContext(ctx, "SELECT name FROM cron_tasks WHERE runtime_version_id = ?", runtimeID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var cname string
			if err := rows.Scan(&cname); err == nil {
				conflicts = append(conflicts, "Cron: "+cname)
			}
		}
	}
	return conflicts, nil
}

func (r *sqliteRepo) HealState(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, "UPDATE runtime_version SET status = 'failed', progress = 0, progress_step = 'failed', error_message = 'Interrupted by server restart' WHERE status = 'installing'")
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, "UPDATE runtime_version SET status = 'uninstall_failed', progress = 0, progress_step = 'uninstall_failed', error_message = 'Interrupted by server restart' WHERE status = 'uninstalling'")
	return err
}
