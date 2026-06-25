package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"easyserver/internal/model"
	"easyserver/internal/repository"
)

// CronRepository implements repository.CronRepository for SQLite
type CronRepository struct {
	db *sql.DB
}

// NewCronRepository creates a new CronRepository
func NewCronRepository(db *sql.DB) repository.CronRepository {
	return &CronRepository{db: db}
}

// ListTasks returns all cron tasks
func (r *CronRepository) ListTasks(ctx context.Context) ([]model.CronTask, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, command, schedule, description,
		 enabled, status, last_run, last_result, next_run,
		 script_id, timeout, max_retry, env_vars, work_dir,
		 created_at, updated_at
		 FROM cron_tasks ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []model.CronTask
	for rows.Next() {
		var t model.CronTask
		var enabled int
		var lastRun, lastResult, nextRun, envVars, workDir sql.NullString
		if err := rows.Scan(
			&t.ID, &t.Name, &t.Command, &t.Schedule, &t.Description,
			&enabled, &t.Status, &lastRun, &lastResult, &nextRun,
			&t.ScriptID, &t.Timeout, &t.MaxRetry, &envVars, &workDir,
			&t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan cron task: %w", err)
		}
		t.Enabled = enabled != 0
		if lastRun.Valid {
			t.LastRun = lastRun.String
		}
		if lastResult.Valid {
			t.LastResult = lastResult.String
		}
		if nextRun.Valid {
			t.NextRun = nextRun.String
		}
		if envVars.Valid {
			t.EnvVars = envVars.String
		}
		if workDir.Valid {
			t.WorkDir = workDir.String
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cron tasks: %w", err)
	}
	return tasks, nil
}

// GetTask returns a cron task by ID
func (r *CronRepository) GetTask(ctx context.Context, id int64) (*model.CronTask, error) {
	var t model.CronTask
	var enabled int
	var lastRun, lastResult, nextRun, envVars, workDir sql.NullString

	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, command, schedule, description,
		 enabled, status, last_run, last_result, next_run,
		 script_id, timeout, max_retry, env_vars, work_dir,
		 created_at, updated_at
		 FROM cron_tasks WHERE id = ?`, id,
	).Scan(
		&t.ID, &t.Name, &t.Command, &t.Schedule, &t.Description,
		&enabled, &t.Status, &lastRun, &lastResult, &nextRun,
		&t.ScriptID, &t.Timeout, &t.MaxRetry, &envVars, &workDir,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	t.Enabled = enabled != 0
	if lastRun.Valid {
		t.LastRun = lastRun.String
	}
	if lastResult.Valid {
		t.LastResult = lastResult.String
	}
	if nextRun.Valid {
		t.NextRun = nextRun.String
	}
	if envVars.Valid {
		t.EnvVars = envVars.String
	}
	if workDir.Valid {
		t.WorkDir = workDir.String
	}
	return &t, nil
}

// CreateTask creates a new cron task
func (r *CronRepository) CreateTask(ctx context.Context, task *model.CronTask) error {
	enabled := 0
	if task.Enabled {
		enabled = 1
	}
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO cron_tasks (name, command, schedule, description, enabled,
		 script_id, timeout, max_retry, env_vars, work_dir)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.Name, task.Command, task.Schedule, task.Description, enabled,
		task.ScriptID, task.Timeout, task.MaxRetry, task.EnvVars, task.WorkDir,
	)
	if err != nil {
		return err
	}
	task.ID, err = result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	return nil
}

// UpdateTask updates an existing cron task
func (r *CronRepository) UpdateTask(ctx context.Context, task *model.CronTask) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE cron_tasks SET name=?, command=?, schedule=?, description=?,
		 script_id=?, timeout=?, max_retry=?, env_vars=?, work_dir=?,
		 updated_at=datetime('now') WHERE id=?`,
		task.Name, task.Command, task.Schedule, task.Description,
		task.ScriptID, task.Timeout, task.MaxRetry, task.EnvVars, task.WorkDir, task.ID,
	)
	return err
}

// DeleteTask deletes a cron task and its logs
func (r *CronRepository) DeleteTask(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "DELETE FROM cron_logs WHERE task_id = ?", id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM cron_tasks WHERE id = ?", id); err != nil {
		return err
	}
	return tx.Commit()
}

// ListEnabledTasks returns all enabled cron tasks
func (r *CronRepository) ListEnabledTasks(ctx context.Context) ([]model.CronTask, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, command, schedule, description,
		 enabled, status, last_run, last_result, next_run,
		 script_id, timeout, max_retry, env_vars, work_dir,
		 created_at, updated_at
		 FROM cron_tasks WHERE enabled = 1 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []model.CronTask
	for rows.Next() {
		var t model.CronTask
		var enabled int
		var lastRun, lastResult, nextRun, envVars, workDir sql.NullString
		if err := rows.Scan(
			&t.ID, &t.Name, &t.Command, &t.Schedule, &t.Description,
			&enabled, &t.Status, &lastRun, &lastResult, &nextRun,
			&t.ScriptID, &t.Timeout, &t.MaxRetry, &envVars, &workDir,
			&t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan cron task: %w", err)
		}
		t.Enabled = enabled != 0
		if lastRun.Valid {
			t.LastRun = lastRun.String
		}
		if lastResult.Valid {
			t.LastResult = lastResult.String
		}
		if nextRun.Valid {
			t.NextRun = nextRun.String
		}
		if envVars.Valid {
			t.EnvVars = envVars.String
		}
		if workDir.Valid {
			t.WorkDir = workDir.String
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cron tasks: %w", err)
	}
	return tasks, nil
}

// EnableTask enables a cron task
func (r *CronRepository) EnableTask(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE cron_tasks SET enabled=1, updated_at=datetime('now') WHERE id=?", id)
	return err
}

// DisableTask disables a cron task
func (r *CronRepository) DisableTask(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE cron_tasks SET enabled=0, updated_at=datetime('now') WHERE id=?", id)
	return err
}

// SetTaskRunning atomically sets a task's status to 'running' if it is not already running.
// Returns true if the status was successfully updated, false if already running.
func (r *CronRepository) SetTaskRunning(ctx context.Context, id int64) (bool, error) {
	result, err := r.db.ExecContext(ctx,
		"UPDATE cron_tasks SET status='running', updated_at=datetime('now') WHERE id=? AND status != 'running'", id)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// UpdateTaskResult updates a task's status, last_run, and last_result
func (r *CronRepository) UpdateTaskResult(ctx context.Context, id int64, status string, lastResult string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE cron_tasks SET status=?, last_run=datetime('now'), last_result=?, updated_at=datetime('now') WHERE id=?`,
		status, lastResult, id)
	return err
}

// CreateLog inserts a new cron execution log
func (r *CronRepository) CreateLog(ctx context.Context, taskID int64, status string, output string, duration int) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO cron_logs (task_id, status, output, duration) VALUES (?, ?, ?, ?)`,
		taskID, status, output, duration)
	return err
}

// GetLogs returns execution logs for a cron task
func (r *CronRepository) GetLogs(ctx context.Context, taskID int64, limit int) ([]model.CronLog, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, task_id, status, output, duration, created_at
		 FROM cron_logs WHERE task_id = ? ORDER BY id DESC LIMIT ?`,
		taskID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []model.CronLog
	for rows.Next() {
		var l model.CronLog
		if err := rows.Scan(&l.ID, &l.TaskID, &l.Status, &l.Output, &l.Duration, &l.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan cron log: %w", err)
		}
		logs = append(logs, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cron logs: %w", err)
	}
	return logs, nil
}

// ListScripts returns all scripts
func (r *CronRepository) ListScripts(ctx context.Context) ([]model.Script, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, description, content, language, created_at, updated_at
		 FROM scripts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var scripts []model.Script
	for rows.Next() {
		var sc model.Script
		if err := rows.Scan(&sc.ID, &sc.Name, &sc.Description, &sc.Content, &sc.Language, &sc.CreatedAt, &sc.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan script: %w", err)
		}
		scripts = append(scripts, sc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate scripts: %w", err)
	}
	return scripts, nil
}

// GetScript returns a script by ID
func (r *CronRepository) GetScript(ctx context.Context, id int64) (*model.Script, error) {
	var sc model.Script
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, description, content, language, created_at, updated_at
		 FROM scripts WHERE id = ?`, id,
	).Scan(&sc.ID, &sc.Name, &sc.Description, &sc.Content, &sc.Language, &sc.CreatedAt, &sc.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &sc, nil
}

// CreateScript creates a new script
func (r *CronRepository) CreateScript(ctx context.Context, script *model.Script) error {
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO scripts (name, description, content, language) VALUES (?, ?, ?, ?)`,
		script.Name, script.Description, script.Content, script.Language,
	)
	if err != nil {
		return err
	}
	script.ID, err = result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	return nil
}

// UpdateScript updates an existing script
func (r *CronRepository) UpdateScript(ctx context.Context, script *model.Script) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE scripts SET name=?, description=?, content=?, language=?, updated_at=datetime('now') WHERE id=?`,
		script.Name, script.Description, script.Content, script.Language, script.ID,
	)
	return err
}

// DeleteScript deletes a script by ID
func (r *CronRepository) DeleteScript(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM scripts WHERE id = ?", id)
	return err
}

// ListDocs returns all documentation sections
func (r *CronRepository) ListDocs(ctx context.Context) ([]model.CronDoc, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, title, content, sort_order, created_at, updated_at FROM cron_docs ORDER BY sort_order")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []model.CronDoc
	for rows.Next() {
		var d model.CronDoc
		if err := rows.Scan(&d.ID, &d.Title, &d.Content, &d.SortOrder, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan cron doc: %w", err)
		}
		docs = append(docs, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cron docs: %w", err)
	}
	return docs, nil
}

// GetDoc returns a documentation section by ID
func (r *CronRepository) GetDoc(ctx context.Context, id int64) (*model.CronDoc, error) {
	var d model.CronDoc
	err := r.db.QueryRowContext(ctx,
		"SELECT id, title, content, sort_order, created_at, updated_at FROM cron_docs WHERE id = ?", id,
	).Scan(&d.ID, &d.Title, &d.Content, &d.SortOrder, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// CreateDoc creates a new documentation section
func (r *CronRepository) CreateDoc(ctx context.Context, doc *model.CronDoc) error {
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO cron_docs (title, content, sort_order) VALUES (?, ?, ?)`,
		doc.Title, doc.Content, doc.SortOrder)
	if err != nil {
		return err
	}
	doc.ID, err = result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}
	return nil
}

// UpdateDoc updates a documentation section
func (r *CronRepository) UpdateDoc(ctx context.Context, doc *model.CronDoc) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE cron_docs SET title=?, content=?, sort_order=?, updated_at=datetime('now') WHERE id=?`,
		doc.Title, doc.Content, doc.SortOrder, doc.ID)
	return err
}

// DeleteDoc deletes a documentation section
func (r *CronRepository) DeleteDoc(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM cron_docs WHERE id = ?", id)
	return err
}

// CountDocs returns the total number of documentation sections
func (r *CronRepository) CountDocs(ctx context.Context) (int, error) {
	var count int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM cron_docs").Scan(&count); err != nil {
		return 0, fmt.Errorf("count cron docs: %w", err)
	}
	return count, nil
}

// BatchCreateDocs inserts multiple documentation sections
func (r *CronRepository) BatchCreateDocs(ctx context.Context, docs []model.CronDoc) error {
	for _, doc := range docs {
		_, err := r.db.ExecContext(ctx,
			`INSERT INTO cron_docs (title, content, sort_order) VALUES (?, ?, ?)`,
			doc.Title, doc.Content, doc.SortOrder)
		if err != nil {
			return err
		}
	}
	return nil
}
