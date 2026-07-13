package deploy

import (
	"context"
	"database/sql"
	"fmt"
	"log"
)

// sqliteRepo implements Repository for SQLite
type sqliteRepo struct {
	db *sql.DB
}

// NewSQLiteRepository creates a new deploy Repository backed by SQLite
func NewSQLiteRepository(db *sql.DB) Repository {
	return &sqliteRepo{db: db}
}

// --- Server CRUD ---

func (r *sqliteRepo) ListServers(ctx context.Context) ([]Server, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id, name, host, port, username, auth_type, status, last_ping, created_at FROM deploy_servers ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []Server
	for rows.Next() {
		var srv Server
		var lastPing sql.NullString
		err := rows.Scan(&srv.ID, &srv.Name, &srv.Host, &srv.Port, &srv.Username, &srv.AuthType, &srv.Status, &lastPing, &srv.CreatedAt)
		if err != nil {
			log.Printf("deploy: scan server row error: %v", err)
			continue
		}
		if lastPing.Valid {
			srv.LastPing = lastPing.String
		}
		servers = append(servers, srv)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate servers: %w", err)
	}

	return servers, nil
}

func (r *sqliteRepo) GetServer(ctx context.Context, id int64) (*Server, error) {
	srv := &Server{}
	var lastPing sql.NullString
	err := r.db.QueryRowContext(ctx,
		"SELECT id, name, host, port, username, auth_type, status, last_ping, created_at FROM deploy_servers WHERE id = ?", id,
	).Scan(&srv.ID, &srv.Name, &srv.Host, &srv.Port, &srv.Username, &srv.AuthType, &srv.Status, &lastPing, &srv.CreatedAt)
	if err != nil {
		return nil, err
	}
	if lastPing.Valid {
		srv.LastPing = lastPing.String
	}
	return srv, nil
}

func (r *sqliteRepo) GetServerAuthData(ctx context.Context, id int64) (string, error) {
	var authData string
	err := r.db.QueryRowContext(ctx,
		"SELECT auth_data FROM deploy_servers WHERE id = ?", id,
	).Scan(&authData)
	if err != nil {
		return "", err
	}
	return authData, nil
}

func (r *sqliteRepo) CreateServer(ctx context.Context, srv *Server) error {
	result, err := r.db.ExecContext(ctx,
		"INSERT INTO deploy_servers (name, host, port, username, auth_type, auth_data) VALUES (?, ?, ?, ?, ?, ?)",
		srv.Name, srv.Host, srv.Port, srv.Username, srv.AuthType, srv.AuthData,
	)
	if err != nil {
		return err
	}
	srv.ID, _ = result.LastInsertId()
	return nil
}

func (r *sqliteRepo) UpdateServer(ctx context.Context, srv *Server) error {
	// Only overwrite auth_data when a new value is supplied. Editing a server's
	// name/host without re-submitting credentials must not wipe the stored,
	// encrypted auth_data (which cannot be round-tripped back from the DB).
	_, err := r.db.ExecContext(ctx,
		`UPDATE deploy_servers SET name=?, host=?, port=?, username=?, auth_type=?,
			auth_data = CASE WHEN ? = '' THEN auth_data ELSE ? END
		 WHERE id=?`,
		srv.Name, srv.Host, srv.Port, srv.Username, srv.AuthType, srv.AuthData, srv.AuthData, srv.ID,
	)
	return err
}

func (r *sqliteRepo) DeleteServer(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM deploy_servers WHERE id=?", id)
	return err
}

func (r *sqliteRepo) UpdateServerStatus(ctx context.Context, id int64, status string, lastPing string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE deploy_servers SET status=?, last_ping=? WHERE id=?", status, lastPing, id)
	return err
}

func (r *sqliteRepo) CountServerTasks(ctx context.Context, serverID int64) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM deploy_tasks WHERE server_id = ?", serverID).Scan(&count)
	return count, err
}

func (r *sqliteRepo) CountServerVersions(ctx context.Context, serverID int64) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM deploy_versions WHERE server_id = ?", serverID).Scan(&count)
	return count, err
}

// --- Task CRUD ---

func (r *sqliteRepo) ListTasks(ctx context.Context) ([]Task, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT t.id, t.server_id, s.name, t.name, t.type, t.source_path, t.dest_path, t.command, t.status, t.result, t.created_at
		FROM deploy_tasks t
		LEFT JOIN deploy_servers s ON t.server_id = s.id
		ORDER BY t.id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var task Task
		var serverName, sourcePath, destPath, command, result sql.NullString
		err := rows.Scan(&task.ID, &task.ServerID, &serverName, &task.Name, &task.Type, &sourcePath, &destPath, &command, &task.Status, &result, &task.CreatedAt)
		if err != nil {
			log.Printf("deploy: scan task row error: %v", err)
			continue
		}
		if serverName.Valid {
			task.ServerName = serverName.String
		}
		if sourcePath.Valid {
			task.SourcePath = sourcePath.String
		}
		if destPath.Valid {
			task.DestPath = destPath.String
		}
		if command.Valid {
			task.Command = command.String
		}
		if result.Valid {
			task.Result = result.String
		}
		tasks = append(tasks, task)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tasks: %w", err)
	}

	return tasks, nil
}

func (r *sqliteRepo) GetTask(ctx context.Context, id int64) (*Task, error) {
	task := &Task{}
	var serverName, sourcePath, destPath, command, result sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT t.id, t.server_id, s.name, t.name, t.type, t.source_path, t.dest_path, t.command, t.status, t.result, t.created_at
		FROM deploy_tasks t
		LEFT JOIN deploy_servers s ON t.server_id = s.id
		WHERE t.id = ?
	`, id).Scan(&task.ID, &task.ServerID, &serverName, &task.Name, &task.Type, &sourcePath, &destPath, &command, &task.Status, &result, &task.CreatedAt)
	if err != nil {
		return nil, err
	}
	if serverName.Valid {
		task.ServerName = serverName.String
	}
	if sourcePath.Valid {
		task.SourcePath = sourcePath.String
	}
	if destPath.Valid {
		task.DestPath = destPath.String
	}
	if command.Valid {
		task.Command = command.String
	}
	if result.Valid {
		task.Result = result.String
	}
	return task, nil
}

func (r *sqliteRepo) ServerExists(ctx context.Context, id int64) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM deploy_servers WHERE id = ?)", id).Scan(&exists)
	return exists, err
}

func (r *sqliteRepo) CreateTask(ctx context.Context, task *Task) error {
	result, err := r.db.ExecContext(ctx,
		"INSERT INTO deploy_tasks (server_id, name, type, source_path, dest_path, command) VALUES (?, ?, ?, ?, ?, ?)",
		task.ServerID, task.Name, task.Type, task.SourcePath, task.DestPath, task.Command,
	)
	if err != nil {
		return err
	}
	task.ID, _ = result.LastInsertId()
	return nil
}

func (r *sqliteRepo) DeleteTask(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM deploy_tasks WHERE id=?", id)
	return err
}

func (r *sqliteRepo) UpdateTaskStatus(ctx context.Context, id int64, status string, result string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE deploy_tasks SET status=?, result=? WHERE id=?", status, result, id)
	return err
}

// TryStartTask atomically transitions a task to "running" only if it is not
// already running, closing the check-then-set race that let concurrent executes
// both run. Returns true if this call won the right to execute.
func (r *sqliteRepo) TryStartTask(ctx context.Context, id int64) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		"UPDATE deploy_tasks SET status='running', result='' WHERE id=? AND status!='running'", id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// --- Version CRUD ---

func (r *sqliteRepo) ListVersions(ctx context.Context, serverID int64) ([]Version, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT v.id, v.server_id, s.name, v.task_id, v.version, v.files, v.backup_path, v.created_at
		FROM deploy_versions v
		LEFT JOIN deploy_servers s ON v.server_id = s.id
		WHERE v.server_id = ?
		ORDER BY v.id DESC
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []Version
	for rows.Next() {
		var ver Version
		var serverName, files, backupPath sql.NullString
		err := rows.Scan(&ver.ID, &ver.ServerID, &serverName, &ver.TaskID, &ver.Version, &files, &backupPath, &ver.CreatedAt)
		if err != nil {
			log.Printf("deploy: scan version row error: %v", err)
			continue
		}
		if serverName.Valid {
			ver.ServerName = serverName.String
		}
		if files.Valid {
			ver.Files = files.String
		}
		if backupPath.Valid {
			ver.BackupPath = backupPath.String
		}
		versions = append(versions, ver)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate versions: %w", err)
	}

	return versions, nil
}

func (r *sqliteRepo) GetVersion(ctx context.Context, id int64) (*Version, error) {
	ver := &Version{}
	var serverName, files, backupPath sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT v.id, v.server_id, s.name, v.task_id, v.version, v.files, v.backup_path, v.created_at
		FROM deploy_versions v
		LEFT JOIN deploy_servers s ON v.server_id = s.id
		WHERE v.id = ?
	`, id).Scan(&ver.ID, &ver.ServerID, &serverName, &ver.TaskID, &ver.Version, &files, &backupPath, &ver.CreatedAt)
	if err != nil {
		return nil, err
	}
	if serverName.Valid {
		ver.ServerName = serverName.String
	}
	if files.Valid {
		ver.Files = files.String
	}
	if backupPath.Valid {
		ver.BackupPath = backupPath.String
	}
	return ver, nil
}

func (r *sqliteRepo) CreateVersion(ctx context.Context, ver *Version) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO deploy_versions (server_id, task_id, version, files, backup_path) VALUES (?, ?, ?, ?, ?)",
		ver.ServerID, ver.TaskID, ver.Version, ver.Files, ver.BackupPath,
	)
	return err
}
