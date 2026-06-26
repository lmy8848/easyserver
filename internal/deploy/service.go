package deploy

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"
)

// Service manages deploy operations
type Service struct {
	repo          Repository
	encryptionKey []byte
}

// NewService creates a new deploy Service
func NewService(repo Repository, encryptionKey string) (*Service, error) {
	if encryptionKey == "" {
		// Allow empty key but don't encrypt
		return &Service{repo: repo}, nil
	}

	if len(encryptionKey) < 32 {
		return nil, fmt.Errorf("deploy encryption key must be at least 32 bytes")
	}

	return &Service{
		repo:          repo,
		encryptionKey: []byte(encryptionKey[:32]),
	}, nil
}

// Server CRUD

func (s *Service) ListServers(ctx context.Context) ([]Server, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.ListServers(ctx)
}

func (s *Service) GetServer(ctx context.Context, id int64) (*Server, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.GetServer(ctx, id)
}

// GetServerAuthData returns the decrypted auth data for internal use only
func (s *Service) GetServerAuthData(ctx context.Context, id int64) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	authData, err := s.repo.GetServerAuthData(ctx, id)
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

func (s *Service) CreateServer(ctx context.Context, srv *Server) error {
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

	srv.AuthData = authData
	return s.repo.CreateServer(ctx, srv)
}

func (s *Service) UpdateServer(ctx context.Context, srv *Server) error {
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

	srv.AuthData = authData
	return s.repo.UpdateServer(ctx, srv)
}

func (s *Service) DeleteServer(ctx context.Context, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	// Check for sub-resources before deleting
	taskCount, err := s.repo.CountServerTasks(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to check tasks: %w", err)
	}
	versionCount, err := s.repo.CountServerVersions(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to check versions: %w", err)
	}

	if taskCount > 0 || versionCount > 0 {
		return fmt.Errorf("server has %d tasks and %d versions; delete them first", taskCount, versionCount)
	}

	return s.repo.DeleteServer(ctx, id)
}

// TestConnection tests SSH connection to a server
func (s *Service) TestConnection(ctx context.Context, id int64) error {
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
		s.repo.UpdateServerStatus(ctx, id, "offline", time.Now().Format(time.RFC3339))
		return fmt.Errorf("failed to get auth data: %w", err)
	}

	log.Printf("deploy: testing connection to %s (%s:%d), auth_type=%s, auth_data_len=%d", srv.Name, srv.Host, srv.Port, srv.AuthType, len(authData))

	// Try SSH connection
	client, err := NewSSHClient(srv, authData)
	if err != nil {
		s.repo.UpdateServerStatus(ctx, id, "offline", time.Now().Format(time.RFC3339))
		return fmt.Errorf("SSH connection failed: %w", err)
	}
	defer client.Close()

	// Connection successful
	s.repo.UpdateServerStatus(ctx, id, "online", time.Now().Format(time.RFC3339))
	log.Printf("deploy: tested connection to %s (%s:%d) - OK", srv.Name, srv.Host, srv.Port)
	return nil
}

// Task CRUD

func (s *Service) ListTasks(ctx context.Context) ([]Task, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.ListTasks(ctx)
}

func (s *Service) GetTask(ctx context.Context, id int64) (*Task, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.GetTask(ctx, id)
}

func (s *Service) CreateTask(ctx context.Context, task *Task) error {
	if ctx == nil {
		ctx = context.Background()
	}
	// Verify server exists
	exists, err := s.repo.ServerExists(ctx, task.ServerID)
	if err != nil {
		return fmt.Errorf("failed to check server: %w", err)
	}
	if !exists {
		return fmt.Errorf("server %d does not exist", task.ServerID)
	}

	return s.repo.CreateTask(ctx, task)
}

func (s *Service) DeleteTask(ctx context.Context, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.DeleteTask(ctx, id)
}

// ExecuteTask executes a deploy task
func (s *Service) ExecuteTask(ctx context.Context, taskID int64) error {
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
	if err := s.repo.UpdateTaskStatus(ctx, taskID, "running", ""); err != nil {
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
		if updateErr := s.repo.UpdateTaskStatus(ctx, taskID, "failed", err.Error()); updateErr != nil {
			log.Printf("deploy: failed to update task status to failed: %v", updateErr)
		}
		return err
	}

	if err := s.repo.UpdateTaskStatus(ctx, taskID, "success", result); err != nil {
		log.Printf("deploy: failed to update task status to success: %v", err)
	}

	// Create version record
	s.createVersion(ctx, task, result)

	return nil
}

func (s *Service) executeSync(srv *Server, task *Task) (string, error) {
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

func (s *Service) executeCommand(srv *Server, task *Task) (string, error) {
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

func (s *Service) executeRollback(srv *Server, task *Task) (string, error) {
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

func (s *Service) createVersion(ctx context.Context, task *Task, result string) {
	version := fmt.Sprintf("v%s", time.Now().Format("20060102-150405"))
	s.repo.CreateVersion(ctx, &Version{
		ServerID: task.ServerID,
		TaskID:   task.ID,
		Version:  version,
		Files:    result,
	})
}

// Version management

func (s *Service) ListVersions(ctx context.Context, serverID int64) ([]Version, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.ListVersions(ctx, serverID)
}

func (s *Service) RollbackVersion(ctx context.Context, versionID int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	// Get version info
	ver, err := s.repo.GetVersion(ctx, versionID)
	if err != nil {
		return fmt.Errorf("version not found: %w", err)
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
	s.repo.CreateVersion(ctx, &Version{
		ServerID:   ver.ServerID,
		TaskID:     ver.TaskID,
		Version:    rollbackVersion,
		Files:      ver.Files,
		BackupPath: ver.BackupPath,
	})

	log.Printf("deploy: rolled back to version %s on server %s", ver.Version, srv.Name)
	return nil
}

// shellEscape escapes a string for safe use in shell commands
func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
