package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"easyserver/internal/model"
	"easyserver/internal/repository"
)

// DeployRepository implements repository.DeployRepository for SQLite
type DeployRepository struct {
	db *sql.DB
}

// NewDeployRepository creates a new DeployRepository
func NewDeployRepository(db *sql.DB) repository.DeployRepository {
	return &DeployRepository{db: db}
}

// --- Server CRUD ---

// ListServers returns all deploy servers
func (r *DeployRepository) ListServers(ctx context.Context) ([]model.DeployServer, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id, name, host, port, username, auth_type, status, last_ping, created_at FROM deploy_servers ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []model.DeployServer
	for rows.Next() {
		var srv model.DeployServer
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

// GetServer returns a deploy server by ID
func (r *DeployRepository) GetServer(ctx context.Context, id int64) (*model.DeployServer, error) {
	srv := &model.DeployServer{}
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

// GetServerAuthData returns the raw (encrypted) auth data for a server
func (r *DeployRepository) GetServerAuthData(ctx context.Context, id int64) (string, error) {
	var authData string
	err := r.db.QueryRowContext(ctx,
		"SELECT auth_data FROM deploy_servers WHERE id = ?", id,
	).Scan(&authData)
	if err != nil {
		return "", err
	}
	return authData, nil
}

// CreateServer inserts a new deploy server
func (r *DeployRepository) CreateServer(ctx context.Context, srv *model.DeployServer) error {
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

// UpdateServer updates an existing deploy server
func (r *DeployRepository) UpdateServer(ctx context.Context, srv *model.DeployServer) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE deploy_servers SET name=?, host=?, port=?, username=?, auth_type=?, auth_data=? WHERE id=?",
		srv.Name, srv.Host, srv.Port, srv.Username, srv.AuthType, srv.AuthData, srv.ID,
	)
	return err
}

// DeleteServer deletes a deploy server by ID
func (r *DeployRepository) DeleteServer(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM deploy_servers WHERE id=?", id)
	return err
}

// UpdateServerStatus updates a server's status and last ping time
func (r *DeployRepository) UpdateServerStatus(ctx context.Context, id int64, status string, lastPing string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE deploy_servers SET status=?, last_ping=? WHERE id=?", status, lastPing, id)
	return err
}

// CountServerTasks returns the number of tasks for a server
func (r *DeployRepository) CountServerTasks(ctx context.Context, serverID int64) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM deploy_tasks WHERE server_id = ?", serverID).Scan(&count)
	return count, err
}

// CountServerVersions returns the number of versions for a server
func (r *DeployRepository) CountServerVersions(ctx context.Context, serverID int64) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM deploy_versions WHERE server_id = ?", serverID).Scan(&count)
	return count, err
}

// --- Task CRUD ---

// ListTasks returns all deploy tasks with server names
func (r *DeployRepository) ListTasks(ctx context.Context) ([]model.DeployTask, error) {
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

	var tasks []model.DeployTask
	for rows.Next() {
		var task model.DeployTask
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

// GetTask returns a deploy task by ID with server name
func (r *DeployRepository) GetTask(ctx context.Context, id int64) (*model.DeployTask, error) {
	task := &model.DeployTask{}
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

// ServerExists checks if a server exists
func (r *DeployRepository) ServerExists(ctx context.Context, id int64) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM deploy_servers WHERE id = ?)", id).Scan(&exists)
	return exists, err
}

// CreateTask inserts a new deploy task
func (r *DeployRepository) CreateTask(ctx context.Context, task *model.DeployTask) error {
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

// DeleteTask deletes a deploy task by ID
func (r *DeployRepository) DeleteTask(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM deploy_tasks WHERE id=?", id)
	return err
}

// UpdateTaskStatus updates a task's status and result
func (r *DeployRepository) UpdateTaskStatus(ctx context.Context, id int64, status string, result string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE deploy_tasks SET status=?, result=? WHERE id=?", status, result, id)
	return err
}

// --- Version CRUD ---

// ListVersions returns all versions for a server
func (r *DeployRepository) ListVersions(ctx context.Context, serverID int64) ([]model.DeployVersion, error) {
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

	var versions []model.DeployVersion
	for rows.Next() {
		var ver model.DeployVersion
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

// GetVersion returns a deploy version by ID
func (r *DeployRepository) GetVersion(ctx context.Context, id int64) (*model.DeployVersion, error) {
	ver := &model.DeployVersion{}
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

// CreateVersion inserts a new deploy version
func (r *DeployRepository) CreateVersion(ctx context.Context, ver *model.DeployVersion) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO deploy_versions (server_id, task_id, version, files, backup_path) VALUES (?, ?, ?, ?, ?)",
		ver.ServerID, ver.TaskID, ver.Version, ver.Files, ver.BackupPath,
	)
	return err
}
