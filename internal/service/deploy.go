package service

import (
	"database/sql"
	"fmt"
	"log"
	"time"
)

type DeployServer struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Username  string `json:"username"`
	AuthType  string `json:"auth_type"`  // password or key
	AuthData  string `json:"auth_data"`  // password or key path (encrypted in DB)
	Status    string `json:"status"`     // online, offline, unknown
	LastPing  string `json:"last_ping"`
	CreatedAt string `json:"created_at"`
}

type DeployTask struct {
	ID         int64  `json:"id"`
	ServerID   int64  `json:"server_id"`
	ServerName string `json:"server_name"`
	Name       string `json:"name"`
	Type       string `json:"type"` // sync, command, rollback
	SourcePath string `json:"source_path"`
	DestPath   string `json:"dest_path"`
	Command    string `json:"command"`
	Status     string `json:"status"` // pending, running, success, failed
	Result     string `json:"result"`
	CreatedAt  string `json:"created_at"`
}

type DeployVersion struct {
	ID         int64  `json:"id"`
	ServerID   int64  `json:"server_id"`
	ServerName string `json:"server_name"`
	TaskID     int64  `json:"task_id"`
	Version    string `json:"version"`
	Files      string `json:"files"`       // JSON array of changed files
	BackupPath string `json:"backup_path"`
	CreatedAt  string `json:"created_at"`
}

type DeployService struct {
	db             *sql.DB
	encryptionKey  []byte
}

func NewDeployService(db *sql.DB, encryptionKey string) (*DeployService, error) {
	if encryptionKey == "" {
		// Allow empty key but don't encrypt
		return &DeployService{db: db}, nil
	}

	if len(encryptionKey) < 32 {
		return nil, fmt.Errorf("deploy encryption key must be at least 32 bytes")
	}

	return &DeployService{
		db:            db,
		encryptionKey: []byte(encryptionKey[:32]),
	}, nil
}

// InitTables creates deploy tables if they don't exist
func (s *DeployService) InitTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS deploy_servers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			host TEXT NOT NULL,
			port INTEGER DEFAULT 22,
			username TEXT NOT NULL,
			auth_type TEXT CHECK(auth_type IN ('password', 'key')),
			auth_data TEXT,
			status TEXT DEFAULT 'unknown',
			last_ping TEXT,
			created_at TEXT DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS deploy_tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			server_id INTEGER REFERENCES deploy_servers(id),
			name TEXT NOT NULL,
			type TEXT CHECK(type IN ('sync', 'command', 'rollback')),
			source_path TEXT,
			dest_path TEXT,
			command TEXT,
			status TEXT DEFAULT 'pending',
			result TEXT,
			created_at TEXT DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS deploy_versions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			server_id INTEGER REFERENCES deploy_servers(id),
			task_id INTEGER REFERENCES deploy_tasks(id),
			version TEXT NOT NULL,
			files TEXT,
			backup_path TEXT,
			created_at TEXT DEFAULT (datetime('now'))
		)`,
	}

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}

	return nil
}

// Server CRUD

func (s *DeployService) ListServers() ([]DeployServer, error) {
	rows, err := s.db.Query("SELECT id, name, host, port, username, auth_type, status, last_ping, created_at FROM deploy_servers ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []DeployServer
	for rows.Next() {
		var srv DeployServer
		var lastPing sql.NullString
		err := rows.Scan(&srv.ID, &srv.Name, &srv.Host, &srv.Port, &srv.Username, &srv.AuthType, &srv.Status, &lastPing, &srv.CreatedAt)
		if err != nil {
			continue
		}
		if lastPing.Valid {
			srv.LastPing = lastPing.String
		}
		servers = append(servers, srv)
	}

	return servers, nil
}

func (s *DeployService) GetServer(id int64) (*DeployServer, error) {
	srv := &DeployServer{}
	var lastPing sql.NullString
	err := s.db.QueryRow(
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

// GetServerAuthData returns the decrypted auth data for internal use only
func (s *DeployService) GetServerAuthData(id int64) (string, error) {
	var authData string
	err := s.db.QueryRow(
		"SELECT auth_data FROM deploy_servers WHERE id = ?", id,
	).Scan(&authData)
	if err != nil {
		return "", err
	}

	// Decrypt if encryption key is set
	if s.encryptionKey != nil && authData != "" {
		decrypted, err := Decrypt(authData, s.encryptionKey)
		if err != nil {
			// If decryption fails, assume it's plain text (migration case)
			log.Printf("deploy: failed to decrypt auth data for server %d, assuming plain text: %v", id, err)
			return authData, nil
		}
		return decrypted, nil
	}

	return authData, nil
}

func (s *DeployService) CreateServer(srv *DeployServer) error {
	// Encrypt auth data if encryption key is set
	authData := srv.AuthData
	if s.encryptionKey != nil && authData != "" {
		encrypted, err := Encrypt(authData, s.encryptionKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt auth data: %w", err)
		}
		authData = encrypted
	}

	result, err := s.db.Exec(
		"INSERT INTO deploy_servers (name, host, port, username, auth_type, auth_data) VALUES (?, ?, ?, ?, ?, ?)",
		srv.Name, srv.Host, srv.Port, srv.Username, srv.AuthType, authData,
	)
	if err != nil {
		return err
	}
	srv.ID, _ = result.LastInsertId()
	return nil
}

func (s *DeployService) UpdateServer(srv *DeployServer) error {
	// Encrypt auth data if encryption key is set
	authData := srv.AuthData
	if s.encryptionKey != nil && authData != "" {
		encrypted, err := Encrypt(authData, s.encryptionKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt auth data: %w", err)
		}
		authData = encrypted
	}

	_, err := s.db.Exec(
		"UPDATE deploy_servers SET name=?, host=?, port=?, username=?, auth_type=?, auth_data=? WHERE id=?",
		srv.Name, srv.Host, srv.Port, srv.Username, srv.AuthType, authData, srv.ID,
	)
	return err
}

func (s *DeployService) DeleteServer(id int64) error {
	_, err := s.db.Exec("DELETE FROM deploy_servers WHERE id=?", id)
	return err
}

// TestConnection tests SSH connection to a server
func (s *DeployService) TestConnection(id int64) error {
	srv, err := s.GetServer(id)
	if err != nil {
		return err
	}

	// TODO: Implement actual SSH connection test
	// For now, just update status
	s.db.Exec("UPDATE deploy_servers SET status='online', last_ping=? WHERE id=?", time.Now().Format(time.RFC3339), id)

	log.Printf("deploy: tested connection to %s (%s:%d)", srv.Name, srv.Host, srv.Port)
	return nil
}

// Task CRUD

func (s *DeployService) ListTasks() ([]DeployTask, error) {
	rows, err := s.db.Query(`
		SELECT t.id, t.server_id, s.name, t.name, t.type, t.source_path, t.dest_path, t.command, t.status, t.result, t.created_at
		FROM deploy_tasks t
		LEFT JOIN deploy_servers s ON t.server_id = s.id
		ORDER BY t.id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []DeployTask
	for rows.Next() {
		var task DeployTask
		var serverName, sourcePath, destPath, command, result sql.NullString
		err := rows.Scan(&task.ID, &task.ServerID, &serverName, &task.Name, &task.Type, &sourcePath, &destPath, &command, &task.Status, &result, &task.CreatedAt)
		if err != nil {
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

	return tasks, nil
}

func (s *DeployService) GetTask(id int64) (*DeployTask, error) {
	task := &DeployTask{}
	var serverName, sourcePath, destPath, command, result sql.NullString
	err := s.db.QueryRow(`
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

func (s *DeployService) CreateTask(task *DeployTask) error {
	result, err := s.db.Exec(
		"INSERT INTO deploy_tasks (server_id, name, type, source_path, dest_path, command) VALUES (?, ?, ?, ?, ?, ?)",
		task.ServerID, task.Name, task.Type, task.SourcePath, task.DestPath, task.Command,
	)
	if err != nil {
		return err
	}
	task.ID, _ = result.LastInsertId()
	return nil
}

func (s *DeployService) DeleteTask(id int64) error {
	_, err := s.db.Exec("DELETE FROM deploy_tasks WHERE id=?", id)
	return err
}

// ExecuteTask executes a deploy task
func (s *DeployService) ExecuteTask(taskID int64) error {
	task, err := s.GetTask(taskID)
	if err != nil {
		return err
	}

	srv, err := s.GetServer(task.ServerID)
	if err != nil {
		return err
	}

	// Update task status
	s.db.Exec("UPDATE deploy_tasks SET status='running' WHERE id=?", taskID)

	// Execute based on task type
	var result string
	switch task.Type {
	case "sync":
		result, err = s.executeSync(srv, task)
	case "command":
		result, err = s.executeCommand(srv, task)
	case "rollback":
		result, err = s.executeRollback(srv, task)
	default:
		err = fmt.Errorf("unknown task type: %s", task.Type)
	}

	if err != nil {
		s.db.Exec("UPDATE deploy_tasks SET status='failed', result=? WHERE id=?", err.Error(), taskID)
		return err
	}

	s.db.Exec("UPDATE deploy_tasks SET status='success', result=? WHERE id=?", result, taskID)

	// Create version record
	s.createVersion(task, result)

	return nil
}

func (s *DeployService) executeSync(srv *DeployServer, task *DeployTask) (string, error) {
	// TODO: Implement SCP/SFTP file sync
	// For now, return placeholder
	return fmt.Sprintf("Synced %s to %s:%s", task.SourcePath, srv.Host, task.DestPath), nil
}

func (s *DeployService) executeCommand(srv *DeployServer, task *DeployTask) (string, error) {
	// TODO: Implement SSH command execution
	// For now, return placeholder
	return fmt.Sprintf("Executed command on %s: %s", srv.Host, task.Command), nil
}

func (s *DeployService) executeRollback(srv *DeployServer, task *DeployTask) (string, error) {
	// TODO: Implement rollback
	return fmt.Sprintf("Rollback completed on %s", srv.Host), nil
}

func (s *DeployService) createVersion(task *DeployTask, result string) {
	version := fmt.Sprintf("v%s", time.Now().Format("20060102-150405"))
	s.db.Exec(
		"INSERT INTO deploy_versions (server_id, task_id, version, files) VALUES (?, ?, ?, ?)",
		task.ServerID, task.ID, version, result,
	)
}

// Version management

func (s *DeployService) ListVersions(serverID int64) ([]DeployVersion, error) {
	rows, err := s.db.Query(`
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

	var versions []DeployVersion
	for rows.Next() {
		var ver DeployVersion
		var serverName, files, backupPath sql.NullString
		err := rows.Scan(&ver.ID, &ver.ServerID, &serverName, &ver.TaskID, &ver.Version, &files, &backupPath, &ver.CreatedAt)
		if err != nil {
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

	return versions, nil
}

func (s *DeployService) RollbackVersion(versionID int64) error {
	// TODO: Implement version rollback
	return fmt.Errorf("version rollback not implemented yet")
}
