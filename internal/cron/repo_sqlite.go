package cron

import (
	"context"
	"database/sql"
	"fmt"
)

// sqliteRepo implements Repository for SQLite
type sqliteRepo struct {
	db *sql.DB
}

// NewSQLiteRepository creates a new SQLite-backed cron Repository
func NewSQLiteRepository(db *sql.DB) Repository {
	return &sqliteRepo{db: db}
}

// selectCronTaskColumns is the canonical projection for cron_tasks, joined
// with runtime_version so RuntimeLang / RuntimeExact arrive populated. Used
// by ListTasks / GetTask / ListEnabledTasks — keep them in sync via this one
// constant + scanCronTask helper, not three near-duplicate blocks.
const selectCronTaskColumns = `SELECT t.id, t.name, t.command, t.schedule, t.description,
	t.enabled, t.status, t.last_run, t.last_result, t.next_run,
	t.script_id, t.timeout, t.max_retry, t.env_vars, t.work_dir,
	t.runtime_version_id, rv.lang, rv.exact,
	t.created_at, t.updated_at
	FROM cron_tasks t
	INNER JOIN runtime_version rv ON t.runtime_version_id = rv.id`

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanCronTask(s rowScanner) (CronTask, error) {
	var t CronTask
	var enabled int
	var lastRun, lastResult, nextRun, envVars, workDir sql.NullString
	if err := s.Scan(
		&t.ID, &t.Name, &t.Command, &t.Schedule, &t.Description,
		&enabled, &t.Status, &lastRun, &lastResult, &nextRun,
		&t.ScriptID, &t.Timeout, &t.MaxRetry, &envVars, &workDir,
		&t.RuntimeVersionID, &t.RuntimeLang, &t.RuntimeExact,
		&t.CreatedAt, &t.UpdatedAt,
	); err != nil {
		return t, err
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
	return t, nil
}

func (r *sqliteRepo) ListTasks(ctx context.Context) ([]CronTask, error) {
	rows, err := r.db.QueryContext(ctx, selectCronTaskColumns+" ORDER BY t.id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []CronTask
	for rows.Next() {
		t, err := scanCronTask(rows)
		if err != nil {
			return nil, fmt.Errorf("scan cron task: %w", err)
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cron tasks: %w", err)
	}
	return tasks, nil
}

func (r *sqliteRepo) GetTask(ctx context.Context, id int64) (*CronTask, error) {
	row := r.db.QueryRowContext(ctx, selectCronTaskColumns+" WHERE t.id = ?", id)
	t, err := scanCronTask(row)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *sqliteRepo) CreateTask(ctx context.Context, task *CronTask) error {
	enabled := 0
	if task.Enabled {
		enabled = 1
	}
	result, err := r.db.ExecContext(ctx,
		`INSERT INTO cron_tasks (name, command, schedule, description, enabled,
		 script_id, timeout, max_retry, env_vars, work_dir, runtime_version_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.Name, task.Command, task.Schedule, task.Description, enabled,
		task.ScriptID, task.Timeout, task.MaxRetry, task.EnvVars, task.WorkDir, task.RuntimeVersionID,
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

func (r *sqliteRepo) UpdateTask(ctx context.Context, task *CronTask) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE cron_tasks SET name=?, command=?, schedule=?, description=?,
		 script_id=?, timeout=?, max_retry=?, env_vars=?, work_dir=?, runtime_version_id=?,
		 updated_at=datetime('now') WHERE id=?`,
		task.Name, task.Command, task.Schedule, task.Description,
		task.ScriptID, task.Timeout, task.MaxRetry, task.EnvVars, task.WorkDir, task.RuntimeVersionID, task.ID,
	)
	return err
}

func (r *sqliteRepo) DeleteTask(ctx context.Context, id int64) error {
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

// GetRuntimeVersionStatus reads the status of a runtime_version row. Used at
// task create-time so we can refuse to bind to an installing/failed runtime
// (which would only blow up later at exec with an opaque mise error).
func (r *sqliteRepo) GetRuntimeVersionStatus(ctx context.Context, runtimeVersionID int64) (string, error) {
	var status string
	err := r.db.QueryRowContext(ctx,
		"SELECT status FROM runtime_version WHERE id = ?", runtimeVersionID).Scan(&status)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("runtime_version %d not found", runtimeVersionID)
	}
	if err != nil {
		return "", err
	}
	return status, nil
}

// GetRuntime 返回 runtime_version 行的 lang/exact/status。
// 供 systemd 包的 ServiceManager.fillRuntime 调用，补全托管 unit 的 mise 包裹参数。
func (r *sqliteRepo) GetRuntime(ctx context.Context, id int64) (lang, exact, status string, err error) {
	err = r.db.QueryRowContext(ctx,
		"SELECT lang, exact, status FROM runtime_version WHERE id = ?", id).
		Scan(&lang, &exact, &status)
	if err == sql.ErrNoRows {
		return "", "", "", fmt.Errorf("runtime_version %d not found", id)
	}
	if err != nil {
		return "", "", "", err
	}
	return lang, exact, status, nil
}

func (r *sqliteRepo) ListEnabledTasks(ctx context.Context) ([]CronTask, error) {
	rows, err := r.db.QueryContext(ctx, selectCronTaskColumns+" WHERE t.enabled = 1 ORDER BY t.id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []CronTask
	for rows.Next() {
		t, err := scanCronTask(rows)
		if err != nil {
			return nil, fmt.Errorf("scan cron task: %w", err)
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cron tasks: %w", err)
	}
	return tasks, nil
}

func (r *sqliteRepo) EnableTask(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE cron_tasks SET enabled=1, updated_at=datetime('now') WHERE id=?", id)
	return err
}

func (r *sqliteRepo) DisableTask(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE cron_tasks SET enabled=0, updated_at=datetime('now') WHERE id=?", id)
	return err
}

func (r *sqliteRepo) SetTaskRunning(ctx context.Context, id int64) (bool, error) {
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

func (r *sqliteRepo) UpdateTaskResult(ctx context.Context, id int64, status string, lastResult string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE cron_tasks SET status=?, last_run=datetime('now'), last_result=?, updated_at=datetime('now') WHERE id=?`,
		status, lastResult, id)
	return err
}

func (r *sqliteRepo) CreateLog(ctx context.Context, taskID int64, status string, output string, duration int) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO cron_logs (task_id, status, output, duration) VALUES (?, ?, ?, ?)`,
		taskID, status, output, duration)
	return err
}

func (r *sqliteRepo) GetLogs(ctx context.Context, taskID int64, limit int) ([]CronLog, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, task_id, status, output, duration, created_at
		 FROM cron_logs WHERE task_id = ? ORDER BY id DESC LIMIT ?`,
		taskID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []CronLog
	for rows.Next() {
		var l CronLog
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

func (r *sqliteRepo) ListScripts(ctx context.Context) ([]Script, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, description, content, language, created_at, updated_at
		 FROM scripts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var scripts []Script
	for rows.Next() {
		var sc Script
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

func (r *sqliteRepo) GetScript(ctx context.Context, id int64) (*Script, error) {
	var sc Script
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, description, content, language, created_at, updated_at
		 FROM scripts WHERE id = ?`, id,
	).Scan(&sc.ID, &sc.Name, &sc.Description, &sc.Content, &sc.Language, &sc.CreatedAt, &sc.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &sc, nil
}

func (r *sqliteRepo) CreateScript(ctx context.Context, script *Script) error {
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

func (r *sqliteRepo) UpdateScript(ctx context.Context, script *Script) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE scripts SET name=?, description=?, content=?, language=?, updated_at=datetime('now') WHERE id=?`,
		script.Name, script.Description, script.Content, script.Language, script.ID,
	)
	return err
}

func (r *sqliteRepo) DeleteScript(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM scripts WHERE id = ?", id)
	return err
}

func (r *sqliteRepo) ListDocs(ctx context.Context) ([]CronDoc, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, title, content, sort_order, created_at, updated_at FROM cron_docs ORDER BY sort_order")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []CronDoc
	for rows.Next() {
		var d CronDoc
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

func (r *sqliteRepo) GetDoc(ctx context.Context, id int64) (*CronDoc, error) {
	var d CronDoc
	err := r.db.QueryRowContext(ctx,
		"SELECT id, title, content, sort_order, created_at, updated_at FROM cron_docs WHERE id = ?", id,
	).Scan(&d.ID, &d.Title, &d.Content, &d.SortOrder, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *sqliteRepo) CreateDoc(ctx context.Context, doc *CronDoc) error {
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

func (r *sqliteRepo) UpdateDoc(ctx context.Context, doc *CronDoc) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE cron_docs SET title=?, content=?, sort_order=?, updated_at=datetime('now') WHERE id=?`,
		doc.Title, doc.Content, doc.SortOrder, doc.ID)
	return err
}

func (r *sqliteRepo) DeleteDoc(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM cron_docs WHERE id = ?", id)
	return err
}

func (r *sqliteRepo) CountDocs(ctx context.Context) (int, error) {
	var count int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM cron_docs").Scan(&count); err != nil {
		return 0, fmt.Errorf("count cron docs: %w", err)
	}
	return count, nil
}

func (r *sqliteRepo) BatchCreateDocs(ctx context.Context, docs []CronDoc) error {
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
