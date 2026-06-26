package process

import (
	"context"
	"database/sql"
	"fmt"
)

// sqliteRepo implements Repository for SQLite
type sqliteRepo struct {
	db *sql.DB
}

// NewSQLiteRepository creates a new SQLite-backed Repository
func NewSQLiteRepository(db *sql.DB) Repository {
	return &sqliteRepo{db: db}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ListProcesses returns all process configurations
func (r *sqliteRepo) ListProcesses(ctx context.Context) ([]Process, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, name, command, args, dir, env, auto_restart, max_restarts,
		 restart_delay, stop_timeout, startup_timeout, auto_start, log_file, group_id, created_at, updated_at
		 FROM processes ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list processes: %w", err)
	}
	defer rows.Close()

	var result []Process
	for rows.Next() {
		p, err := scanProcess(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

// GetProcessByID returns a single process by ID, nil if not found
func (r *sqliteRepo) GetProcessByID(ctx context.Context, id int64) (*Process, error) {
	var p Process
	var autoRestart, autoStart int
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, command, args, dir, env, auto_restart, max_restarts,
		 restart_delay, stop_timeout, startup_timeout, auto_start, log_file, group_id, created_at, updated_at
		 FROM processes WHERE id = ?`, id,
	).Scan(
		&p.ID, &p.Name, &p.Command, &p.Args, &p.Dir, &p.Env,
		&autoRestart, &p.MaxRestarts, &p.RestartDelay,
		&p.StopTimeout, &p.StartupTimeout, &autoStart,
		&p.LogFile, &p.GroupID, &p.CreatedAt, &p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get process: %w", err)
	}
	p.AutoRestart = autoRestart != 0
	p.AutoStart = autoStart != 0
	return &p, nil
}

// CreateProcess inserts a new process configuration
func (r *sqliteRepo) CreateProcess(ctx context.Context, p *Process) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx,
		`INSERT INTO processes (name, command, args, dir, env, auto_restart, max_restarts,
		 restart_delay, stop_timeout, startup_timeout, auto_start, log_file, group_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Name, p.Command, p.Args, p.Dir, p.Env,
		boolToInt(p.AutoRestart), p.MaxRestarts, p.RestartDelay,
		p.StopTimeout, p.StartupTimeout,
		boolToInt(p.AutoStart), p.LogFile, p.GroupID,
	)
	if err != nil {
		return 0, fmt.Errorf("create process: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO process_status (process_id, status) VALUES (?, 'stopped')`, id); err != nil {
		return 0, err
	}

	return id, tx.Commit()
}

// UpdateProcess performs a partial update on a process
func (r *sqliteRepo) UpdateProcess(ctx context.Context, id int64, req *UpdateProcessRequest) error {
	if req.Name != nil {
		if _, err := r.db.ExecContext(ctx, "UPDATE processes SET name = ?, updated_at = datetime('now') WHERE id = ?", *req.Name, id); err != nil {
			return err
		}
	}
	if req.Command != nil {
		if _, err := r.db.ExecContext(ctx, "UPDATE processes SET command = ?, updated_at = datetime('now') WHERE id = ?", *req.Command, id); err != nil {
			return err
		}
	}
	if req.Args != nil {
		if _, err := r.db.ExecContext(ctx, "UPDATE processes SET args = ?, updated_at = datetime('now') WHERE id = ?", *req.Args, id); err != nil {
			return err
		}
	}
	if req.Dir != nil {
		if _, err := r.db.ExecContext(ctx, "UPDATE processes SET dir = ?, updated_at = datetime('now') WHERE id = ?", *req.Dir, id); err != nil {
			return err
		}
	}
	if req.Env != nil {
		if _, err := r.db.ExecContext(ctx, "UPDATE processes SET env = ?, updated_at = datetime('now') WHERE id = ?", *req.Env, id); err != nil {
			return err
		}
	}
	if req.AutoRestart != nil {
		if _, err := r.db.ExecContext(ctx, "UPDATE processes SET auto_restart = ?, updated_at = datetime('now') WHERE id = ?", boolToInt(*req.AutoRestart), id); err != nil {
			return err
		}
	}
	if req.MaxRestarts != nil {
		if _, err := r.db.ExecContext(ctx, "UPDATE processes SET max_restarts = ?, updated_at = datetime('now') WHERE id = ?", *req.MaxRestarts, id); err != nil {
			return err
		}
	}
	if req.RestartDelay != nil {
		if _, err := r.db.ExecContext(ctx, "UPDATE processes SET restart_delay = ?, updated_at = datetime('now') WHERE id = ?", *req.RestartDelay, id); err != nil {
			return err
		}
	}
	if req.StopTimeout != nil {
		if _, err := r.db.ExecContext(ctx, "UPDATE processes SET stop_timeout = ?, updated_at = datetime('now') WHERE id = ?", *req.StopTimeout, id); err != nil {
			return err
		}
	}
	if req.StartupTimeout != nil {
		if _, err := r.db.ExecContext(ctx, "UPDATE processes SET startup_timeout = ?, updated_at = datetime('now') WHERE id = ?", *req.StartupTimeout, id); err != nil {
			return err
		}
	}
	if req.AutoStart != nil {
		if _, err := r.db.ExecContext(ctx, "UPDATE processes SET auto_start = ?, updated_at = datetime('now') WHERE id = ?", boolToInt(*req.AutoStart), id); err != nil {
			return err
		}
	}
	if req.LogFile != nil {
		if _, err := r.db.ExecContext(ctx, "UPDATE processes SET log_file = ?, updated_at = datetime('now') WHERE id = ?", *req.LogFile, id); err != nil {
			return err
		}
	}
	if req.GroupID != nil {
		if _, err := r.db.ExecContext(ctx, "UPDATE processes SET group_id = ?, updated_at = datetime('now') WHERE id = ?", *req.GroupID, id); err != nil {
			return err
		}
	}
	return nil
}

// DeleteProcess removes a process (cascade handles status and logs)
func (r *sqliteRepo) DeleteProcess(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM processes WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete process: %w", err)
	}
	return nil
}

// GetAutoStartIDs returns IDs of processes with auto_start enabled
func (r *sqliteRepo) GetAutoStartIDs(ctx context.Context) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id FROM processes WHERE auto_start = 1")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// UpsertStatus creates or updates the runtime status of a process
func (r *sqliteRepo) UpsertStatus(ctx context.Context, processID int64, status string, pid int, exitCode int, lastError string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO process_status (process_id, status, pid, exit_code, last_error, last_start, updated_at)
		VALUES (?, ?, ?, ?, ?, datetime('now'), datetime('now'))
		ON CONFLICT(process_id) DO UPDATE SET
			status = excluded.status,
			pid = excluded.pid,
			exit_code = excluded.exit_code,
			last_error = excluded.last_error,
			last_start = CASE WHEN excluded.status = 'running' THEN datetime('now') ELSE last_start END,
			updated_at = datetime('now')`,
		processID, status, pid, exitCode, lastError)
	return err
}

// GetStatus returns the runtime status of a process, nil if not found
func (r *sqliteRepo) GetStatus(ctx context.Context, processID int64) (*ProcessStatus, error) {
	var s ProcessStatus
	err := r.db.QueryRowContext(ctx,
		`SELECT id, process_id, status, pid, uptime, restarts, cpu_percent, memory_mb,
		 exit_code, COALESCE(last_start,''), COALESCE(last_error,''), updated_at
		 FROM process_status WHERE process_id = ?`, processID,
	).Scan(&s.ID, &s.ProcessID, &s.Status, &s.PID, &s.Uptime, &s.Restarts,
		&s.CPUPercent, &s.MemoryMB, &s.ExitCode, &s.LastStart, &s.LastError, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// IncrementRestarts increments the restart counter for a process
func (r *sqliteRepo) IncrementRestarts(ctx context.Context, processID int64) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE process_status SET restarts = restarts + 1 WHERE process_id = ?", processID)
	return err
}

// ClearExitInfo resets exit code and last error for a process
func (r *sqliteRepo) ClearExitInfo(ctx context.Context, processID int64) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE process_status SET exit_code = 0, last_error = '' WHERE process_id = ?", processID)
	return err
}

// AppendLog inserts a log entry for a process
func (r *sqliteRepo) AppendLog(ctx context.Context, processID int64, logType, content string) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO process_logs (process_id, type, content) VALUES (?, ?, ?)",
		processID, logType, content)
	return err
}

// ListLogs returns log entries for a process with pagination
func (r *sqliteRepo) ListLogs(ctx context.Context, processID int64, limit, offset int) ([]ProcessLog, int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM process_logs WHERE process_id = ?", processID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, process_id, type, content, created_at
		 FROM process_logs WHERE process_id = ?
		 ORDER BY id DESC LIMIT ? OFFSET ?`, processID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []ProcessLog
	for rows.Next() {
		var l ProcessLog
		if err := rows.Scan(&l.ID, &l.ProcessID, &l.Type, &l.Content, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		logs = append(logs, l)
	}
	return logs, total, rows.Err()
}

// ListGroups returns all process groups
func (r *sqliteRepo) ListGroups(ctx context.Context) ([]ProcessGroup, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, name, description, created_at FROM process_groups ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []ProcessGroup
	for rows.Next() {
		var g ProcessGroup
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.CreatedAt); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

// GetGroup returns a process group by ID, nil if not found
func (r *sqliteRepo) GetGroup(ctx context.Context, id int64) (*ProcessGroup, error) {
	var g ProcessGroup
	err := r.db.QueryRowContext(ctx,
		"SELECT id, name, description, created_at FROM process_groups WHERE id = ?", id,
	).Scan(&g.ID, &g.Name, &g.Description, &g.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// CreateGroup inserts a new process group
func (r *sqliteRepo) CreateGroup(ctx context.Context, name, description string) (int64, error) {
	result, err := r.db.ExecContext(ctx,
		"INSERT INTO process_groups (name, description) VALUES (?, ?)", name, description)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// UpdateGroup performs a partial update on a process group
func (r *sqliteRepo) UpdateGroup(ctx context.Context, id int64, req *UpdateProcessGroupRequest) error {
	if req.Name != nil {
		if _, err := r.db.ExecContext(ctx, "UPDATE process_groups SET name = ? WHERE id = ?", *req.Name, id); err != nil {
			return err
		}
	}
	if req.Description != nil {
		if _, err := r.db.ExecContext(ctx, "UPDATE process_groups SET description = ? WHERE id = ?", *req.Description, id); err != nil {
			return err
		}
	}
	return nil
}

// DeleteGroup unlinks processes and deletes a process group
func (r *sqliteRepo) DeleteGroup(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "UPDATE processes SET group_id = 0 WHERE group_id = ?", id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM process_groups WHERE id = ?", id); err != nil {
		return err
	}
	return tx.Commit()
}

// scanProcess scans a process row from a *sql.Rows
func scanProcess(rows *sql.Rows) (Process, error) {
	var p Process
	var autoRestart, autoStart int
	if err := rows.Scan(
		&p.ID, &p.Name, &p.Command, &p.Args, &p.Dir, &p.Env,
		&autoRestart, &p.MaxRestarts, &p.RestartDelay,
		&p.StopTimeout, &p.StartupTimeout, &autoStart,
		&p.LogFile, &p.GroupID, &p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		return Process{}, fmt.Errorf("scan process: %w", err)
	}
	p.AutoRestart = autoRestart != 0
	p.AutoStart = autoStart != 0
	return p, nil
}
