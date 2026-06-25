package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"path/filepath"
	"strings"
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

// Server CRUD

func (s *DeployService) ListServers(ctx context.Context) ([]DeployServer, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := s.db.QueryContext(ctx, "SELECT id, name, host, port, username, auth_type, status, last_ping, created_at FROM deploy_servers ORDER BY id")
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

func (s *DeployService) GetServer(ctx context.Context, id int64) (*DeployServer, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	srv := &DeployServer{}
	var lastPing sql.NullString
	err := s.db.QueryRowContext(ctx,
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
func (s *DeployService) GetServerAuthData(ctx context.Context, id int64) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var authData string
	err := s.db.QueryRowContext(ctx,
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

func (s *DeployService) CreateServer(ctx context.Context, srv *DeployServer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	// Encrypt auth data if encryption key is set
	authData := srv.AuthData
	if s.encryptionKey != nil && authData != "" {
		encrypted, err := Encrypt(authData, s.encryptionKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt auth data: %w", err)
		}
		authData = encrypted
	}

	result, err := s.db.ExecContext(ctx,
		"INSERT INTO deploy_servers (name, host, port, username, auth_type, auth_data) VALUES (?, ?, ?, ?, ?, ?)",
		srv.Name, srv.Host, srv.Port, srv.Username, srv.AuthType, authData,
	)
	if err != nil {
		return err
	}
	srv.ID, _ = result.LastInsertId()
	return nil
}

func (s *DeployService) UpdateServer(ctx context.Context, srv *DeployServer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	// Encrypt auth data if encryption key is set
	authData := srv.AuthData
	if s.encryptionKey != nil && authData != "" {
		encrypted, err := Encrypt(authData, s.encryptionKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt auth data: %w", err)
		}
		authData = encrypted
	}

	_, err := s.db.ExecContext(ctx,
		"UPDATE deploy_servers SET name=?, host=?, port=?, username=?, auth_type=?, auth_data=? WHERE id=?",
		srv.Name, srv.Host, srv.Port, srv.Username, srv.AuthType, authData, srv.ID,
	)
	return err
}

func (s *DeployService) DeleteServer(ctx context.Context, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	// Check for sub-resources before deleting
	var taskCount int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM deploy_tasks WHERE server_id = ?", id).Scan(&taskCount); err != nil {
		return fmt.Errorf("failed to check tasks: %w", err)
	}
	var versionCount int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM deploy_versions WHERE server_id = ?", id).Scan(&versionCount); err != nil {
		return fmt.Errorf("failed to check versions: %w", err)
	}

	if taskCount > 0 || versionCount > 0 {
		return fmt.Errorf("server has %d tasks and %d versions; delete them first", taskCount, versionCount)
	}

	_, err := s.db.ExecContext(ctx, "DELETE FROM deploy_servers WHERE id=?", id)
	return err
}

// TestConnection tests SSH connection to a server
func (s *DeployService) TestConnection(ctx context.Context, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	srv, err := s.GetServer(ctx, id)
	if err != nil {
		return err
	}

	// Get auth data
	authData, err := s.GetServerAuthData(ctx, id)
	if err != nil {
		s.db.ExecContext(ctx, "UPDATE deploy_servers SET status='offline', last_ping=? WHERE id=?", time.Now().Format(time.RFC3339), id)
		return fmt.Errorf("failed to get auth data: %w", err)
	}

	log.Printf("deploy: testing connection to %s (%s:%d), auth_type=%s, auth_data_len=%d", srv.Name, srv.Host, srv.Port, srv.AuthType, len(authData))

	// Try SSH connection
	client, err := NewSSHClient(srv, authData)
	if err != nil {
		s.db.ExecContext(ctx, "UPDATE deploy_servers SET status='offline', last_ping=? WHERE id=?", time.Now().Format(time.RFC3339), id)
		return fmt.Errorf("SSH connection failed: %w", err)
	}
	defer client.Close()

	// Connection successful
	s.db.ExecContext(ctx, "UPDATE deploy_servers SET status='online', last_ping=? WHERE id=?", time.Now().Format(time.RFC3339), id)
	log.Printf("deploy: tested connection to %s (%s:%d) - OK", srv.Name, srv.Host, srv.Port)
	return nil
}

// Task CRUD

func (s *DeployService) ListTasks(ctx context.Context) ([]DeployTask, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := s.db.QueryContext(ctx, `
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

func (s *DeployService) GetTask(ctx context.Context, id int64) (*DeployTask, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	task := &DeployTask{}
	var serverName, sourcePath, destPath, command, result sql.NullString
	err := s.db.QueryRowContext(ctx, `
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

func (s *DeployService) CreateTask(ctx context.Context, task *DeployTask) error {
	if ctx == nil {
		ctx = context.Background()
	}
	// Verify server exists
	var exists bool
	if err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM deploy_servers WHERE id = ?)", task.ServerID).Scan(&exists); err != nil {
		return fmt.Errorf("failed to check server: %w", err)
	}
	if !exists {
		return fmt.Errorf("server %d does not exist", task.ServerID)
	}

	result, err := s.db.ExecContext(ctx,
		"INSERT INTO deploy_tasks (server_id, name, type, source_path, dest_path, command) VALUES (?, ?, ?, ?, ?, ?)",
		task.ServerID, task.Name, task.Type, task.SourcePath, task.DestPath, task.Command,
	)
	if err != nil {
		return err
	}
	task.ID, _ = result.LastInsertId()
	return nil
}

func (s *DeployService) DeleteTask(ctx context.Context, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	_, err := s.db.ExecContext(ctx, "DELETE FROM deploy_tasks WHERE id=?", id)
	return err
}

// ExecuteTask executes a deploy task
func (s *DeployService) ExecuteTask(ctx context.Context, taskID int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return err
	}

	// Guard: reject if task is already running
	if task.Status == "running" {
		return fmt.Errorf("task %d is already running", taskID)
	}

	srv, err := s.GetServer(ctx, task.ServerID)
	if err != nil {
		return err
	}

	// Update task status
	if _, err := s.db.ExecContext(ctx, "UPDATE deploy_tasks SET status='running' WHERE id=?", taskID); err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}

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
		if _, updateErr := s.db.ExecContext(ctx, "UPDATE deploy_tasks SET status='failed', result=? WHERE id=?", err.Error(), taskID); updateErr != nil {
			log.Printf("deploy: failed to update task status to failed: %v", updateErr)
		}
		return err
	}

	if _, err := s.db.ExecContext(ctx, "UPDATE deploy_tasks SET status='success', result=? WHERE id=?", result, taskID); err != nil {
		log.Printf("deploy: failed to update task status to success: %v", err)
	}

	// Create version record
	s.createVersion(task, result)

	return nil
}

func (s *DeployService) executeSync(srv *DeployServer, task *DeployTask) (string, error) {
	// Validate inputs
	if task.SourcePath == "" {
		return "", fmt.Errorf("source path is required")
	}
	if task.DestPath == "" {
		return "", fmt.Errorf("destination path is required")
	}

	// Get auth data
	authData, err := s.GetServerAuthData(context.Background(), srv.ID)
	if err != nil {
		return "", fmt.Errorf("failed to get auth data: %w", err)
	}

	// Create SSH client
	client, err := NewSSHClient(srv, authData)
	if err != nil {
		return "", fmt.Errorf("SSH connection failed: %w", err)
	}
	defer client.Close()

	// Ensure remote directory exists (escape path to prevent injection)
	safePath := shellEscape(task.DestPath)
	mkdirCmd := fmt.Sprintf("mkdir -p %s", safePath)
	_, _, _, err = client.RunCommand(mkdirCmd, 10*time.Second)
	if err != nil {
		return "", fmt.Errorf("failed to create remote directory: %w", err)
	}

	// Upload file
	err = client.UploadFile(task.SourcePath, task.DestPath)
	if err != nil {
		return "", fmt.Errorf("file upload failed: %w", err)
	}

	result := fmt.Sprintf("Uploaded %s to %s:%s", task.SourcePath, srv.Host, task.DestPath)
	log.Printf("deploy: %s", result)
	return result, nil
}

func (s *DeployService) executeCommand(srv *DeployServer, task *DeployTask) (string, error) {
	// Validate inputs
	if task.Command == "" {
		return "", fmt.Errorf("command is required")
	}
	// Reject null bytes to prevent injection
	if strings.ContainsRune(task.Command, '\x00') {
		return "", fmt.Errorf("command contains null byte")
	}
	// Enforce max command length
	const maxCmdLen = 8192
	if len(task.Command) > maxCmdLen {
		return "", fmt.Errorf("command exceeds maximum length (%d bytes)", maxCmdLen)
	}

	// Get auth data
	authData, err := s.GetServerAuthData(context.Background(), srv.ID)
	if err != nil {
		return "", fmt.Errorf("failed to get auth data: %w", err)
	}

	// Create SSH client
	client, err := NewSSHClient(srv, authData)
	if err != nil {
		return "", fmt.Errorf("SSH connection failed: %w", err)
	}
	defer client.Close()

	// Execute command with 5 minute timeout
	stdout, stderr, exitCode, err := client.RunCommand(task.Command, 5*time.Minute)
	if err != nil {
		return "", fmt.Errorf("command failed (exit code %d): %s %s", exitCode, stdout, stderr)
	}

	result := stdout
	if stderr != "" {
		result += "\n[stderr]\n" + stderr
	}

	log.Printf("deploy: executed command on %s (exit code %d)", srv.Host, exitCode)
	return result, nil
}

func (s *DeployService) executeRollback(srv *DeployServer, task *DeployTask) (string, error) {
	// Get auth data
	authData, err := s.GetServerAuthData(context.Background(), srv.ID)
	if err != nil {
		return "", fmt.Errorf("failed to get auth data: %w", err)
	}

	// Create SSH client
	client, err := NewSSHClient(srv, authData)
	if err != nil {
		return "", fmt.Errorf("SSH connection failed: %w", err)
	}
	defer client.Close()

	// Find the version to rollback to
	// For now, just execute the command if provided
	if task.Command != "" {
		stdout, stderr, exitCode, err := client.RunCommand(task.Command, 5*time.Minute)
		if err != nil {
			return "", fmt.Errorf("rollback command failed (exit code %d): %s %s", exitCode, stdout, stderr)
		}
		result := stdout
		if stderr != "" {
			result += "\n[stderr]\n" + stderr
		}
		log.Printf("deploy: rollback completed on %s (exit code %d)", srv.Host, exitCode)
		return result, nil
	}

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

func (s *DeployService) ListVersions(ctx context.Context, serverID int64) ([]DeployVersion, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := s.db.QueryContext(ctx, `
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

func (s *DeployService) RollbackVersion(ctx context.Context, versionID int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	// Get version info
	var ver DeployVersion
	var serverName, files, backupPath sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT v.id, v.server_id, s.name, v.task_id, v.version, v.files, v.backup_path
		FROM deploy_versions v
		LEFT JOIN deploy_servers s ON v.server_id = s.id
		WHERE v.id = ?
	`, versionID).Scan(&ver.ID, &ver.ServerID, &serverName, &ver.TaskID, &ver.Version, &files, &backupPath)
	if err != nil {
		return fmt.Errorf("version not found: %w", err)
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

	// Get server info
	srv, err := s.GetServer(ctx, ver.ServerID)
	if err != nil {
		return fmt.Errorf("server not found: %w", err)
	}

	// Get auth data
	authData, err := s.GetServerAuthData(ctx, ver.ServerID)
	if err != nil {
		return fmt.Errorf("failed to get auth data: %w", err)
	}

	// Create SSH client
	client, err := NewSSHClient(srv, authData)
	if err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}
	defer client.Close()

	// If there's a backup path, restore from it
	if ver.BackupPath != "" {
		// Validate backup path to prevent path injection
		dangerousPaths := []string{"/", "/*", "/etc", "/usr", "/var", "/bin", "/sbin", "/boot", "/dev", "/proc", "/sys", "/root", "/home"}
		for _, dp := range dangerousPaths {
			if ver.BackupPath == dp || strings.HasPrefix(ver.BackupPath, dp+"/") {
				return fmt.Errorf("refusing to restore from dangerous backup path: %s", ver.BackupPath)
			}
		}
		// Restore from backup - copy files back to the original location
		// Validate and clean the backup path to prevent path traversal
		cleanPath := filepath.Clean(ver.BackupPath)
		if cleanPath == "." || cleanPath == "/" || strings.Contains(cleanPath, "..") {
			return fmt.Errorf("invalid backup path: %s", ver.BackupPath)
		}
		safePath := shellEscape(cleanPath)
		restoreCmd := fmt.Sprintf("cp -r %s/* /", safePath)
		_, stderr, exitCode, err := client.RunCommand(restoreCmd, 5*time.Minute)
		if err != nil {
			return fmt.Errorf("restore failed (exit code %d): %s", exitCode, stderr)
		}
		log.Printf("deploy: restored version %s from backup %s", ver.Version, ver.BackupPath)
	} else if ver.Files != "" {
		// If no backup, just log that rollback is limited
		log.Printf("deploy: version %s has no backup, rollback limited", ver.Version)
	}

	// Create a new version record for the rollback
	rollbackVersion := fmt.Sprintf("rollback-%s-%s", ver.Version, time.Now().Format("20060102-150405"))
	s.db.ExecContext(ctx,
		"INSERT INTO deploy_versions (server_id, task_id, version, files, backup_path) VALUES (?, ?, ?, ?, ?)",
		ver.ServerID, ver.TaskID, rollbackVersion, ver.Files, ver.BackupPath,
	)

	log.Printf("deploy: rolled back to version %s on server %s", ver.Version, srv.Name)
	return nil
}

// shellEscape escapes a string for safe use in shell commands
func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
