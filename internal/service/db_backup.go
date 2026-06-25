package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"easyserver/internal/executor"
	"easyserver/internal/model"
)

const (
	DefaultBackupDir = "/var/backups/easyserver/db"
	DefaultRedisRDB  = "/var/lib/redis/dump.rdb"
	MaxBackupsPerDB  = 10
)

// DBBackupService handles database backup and restore operations
type DBBackupService struct {
	db        *sql.DB
	backupDir string
	executor  executor.CommandExecutor
}

// NewDBBackupService creates a new DBBackupService
func NewDBBackupService(db *sql.DB, exec executor.CommandExecutor) *DBBackupService {
	return &DBBackupService{
		db:        db,
		backupDir: DefaultBackupDir,
		executor:  exec,
	}
}

// SetBackupDir sets the backup directory
func (s *DBBackupService) SetBackupDir(dir string) {
	s.backupDir = dir
}

// Deprecated: InitTables is kept for backward compatibility only.
// Table creation is now handled by the migration system (migrations/ directory).
func (s *DBBackupService) InitTables(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	query := `CREATE TABLE IF NOT EXISTS db_backups (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		db_server_id INTEGER NOT NULL,
		db_version_id INTEGER NOT NULL,
		database_id INTEGER DEFAULT 0,
		database_name TEXT NOT NULL,
		backup_type TEXT NOT NULL DEFAULT 'manual',
		file_path TEXT NOT NULL,
		file_size INTEGER DEFAULT 0,
		status TEXT DEFAULT 'completed',
		error_message TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`
	_, err := s.db.ExecContext(ctx, query)
	return err
}

// CreateBackup creates a backup of a database
func (s *DBBackupService) CreateBackup(ctx context.Context, dbServerID, dbVersionID, databaseID int64, dbName, dbType string) (*model.DBBackup, error) {
	// Ensure backup directory exists
	if err := os.MkdirAll(s.backupDir, 0755); err != nil {
		return nil, fmt.Errorf("create backup dir: %w", err)
	}

	// Generate backup filename
	timestamp := time.Now().Format("20060102150405")
	var fileName string
	switch dbType {
	case "mysql":
		fileName = fmt.Sprintf("%s_%s.sql", dbName, timestamp)
	case "postgresql":
		fileName = fmt.Sprintf("%s_%s.sql", dbName, timestamp)
	case "redis":
		fileName = fmt.Sprintf("dump_%s.rdb", timestamp)
	default:
		return nil, fmt.Errorf("unsupported db type: %s", dbType)
	}
	filePath := filepath.Join(s.backupDir, fileName)

	// Create backup record
	backup := &model.DBBackup{
		DBServerID:   dbServerID,
		DBVersionID:  dbVersionID,
		DatabaseID:   databaseID,
		DatabaseName: dbName,
		BackupType:   "manual",
		FilePath:     filePath,
		Status:       "pending",
	}

	result, err := s.db.ExecContext(ctx,
		`INSERT INTO db_backups (db_server_id, db_version_id, database_id, database_name, backup_type, file_path, status)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		backup.DBServerID, backup.DBVersionID, backup.DatabaseID, backup.DatabaseName, backup.BackupType, backup.FilePath, backup.Status)
	if err != nil {
		return nil, fmt.Errorf("insert backup record: %w", err)
	}
	backup.ID, _ = result.LastInsertId()

	// Execute backup in background with a detached context (request context
	// would be cancelled when the HTTP handler returns, killing the backup).
	backupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	go func() {
		defer cancel()
		s.executeBackup(backupCtx, backup, dbType)
	}()

	return backup, nil
}

// executeBackup performs the actual backup operation
func (s *DBBackupService) executeBackup(ctx context.Context, backup *model.DBBackup, dbType string) {
	var err error

	switch dbType {
	case "mysql":
		err = s.backupMySQL(ctx, backup)
	case "postgresql":
		err = s.backupPostgreSQL(ctx, backup)
	case "redis":
		err = s.backupRedis(ctx, backup)
	}

	if err != nil {
		backup.Status = "failed"
		backup.ErrorMessage = err.Error()
		log.Printf("backup failed for %s: %v", backup.DatabaseName, err)
	} else {
		backup.Status = "completed"
		// Get file size
		if info, err := os.Stat(backup.FilePath); err == nil {
			backup.FileSize = info.Size()
		}
	}

	// Update backup record
	if _, err := s.db.ExecContext(ctx,
		`UPDATE db_backups SET status = ?, file_size = ?, error_message = ? WHERE id = ?`,
		backup.Status, backup.FileSize, backup.ErrorMessage, backup.ID); err != nil {
		log.Printf("failed to update backup record %d: %v", backup.ID, err)
	}
}

// backupMySQL creates a MySQL backup using mysqldump
func (s *DBBackupService) backupMySQL(ctx context.Context, backup *model.DBBackup) error {
	out, _, _, err := s.executor.Run(ctx, "mysqldump", "--single-transaction", "--routines", "--triggers", backup.DatabaseName)
	if err != nil {
		return fmt.Errorf("mysqldump failed: %w", err)
	}
	return os.WriteFile(backup.FilePath, []byte(out), 0644)
}

// backupPostgreSQL creates a PostgreSQL backup using pg_dump
func (s *DBBackupService) backupPostgreSQL(ctx context.Context, backup *model.DBBackup) error {
	out, _, _, err := s.executor.Run(ctx, "sudo", "-u", "postgres", "pg_dump", "-Fc", backup.DatabaseName)
	if err != nil {
		return fmt.Errorf("pg_dump failed: %w", err)
	}
	return os.WriteFile(backup.FilePath, []byte(out), 0644)
}

// backupRedis creates a Redis backup by copying RDB file
func (s *DBBackupService) backupRedis(ctx context.Context, backup *model.DBBackup) error {
	// Trigger BGSAVE
	_, _, err := s.executor.RunCombined(ctx, "redis-cli", "BGSAVE")
	if err != nil {
		return fmt.Errorf("redis BGSAVE failed: %w", err)
	}

	// Wait for BGSAVE to complete
	time.Sleep(2 * time.Second)

	// Copy RDB file
	data, err := os.ReadFile(DefaultRedisRDB)
	if err != nil {
		return fmt.Errorf("read RDB file: %w", err)
	}
	return os.WriteFile(backup.FilePath, data, 0644)
}

// ListBackups returns all backups for a database
func (s *DBBackupService) ListBackups(ctx context.Context, databaseID int64) ([]model.DBBackup, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, db_server_id, db_version_id, database_id, database_name, backup_type, file_path, file_size, status, error_message, created_at
		FROM db_backups WHERE database_id = ? ORDER BY created_at DESC`, databaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var backups []model.DBBackup
	for rows.Next() {
		var b model.DBBackup
		if err := rows.Scan(&b.ID, &b.DBServerID, &b.DBVersionID, &b.DatabaseID, &b.DatabaseName, &b.BackupType, &b.FilePath, &b.FileSize, &b.Status, &b.ErrorMessage, &b.CreatedAt); err != nil {
			log.Printf("scan backup row: %v", err)
			continue
		}
		backups = append(backups, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate backups: %w", err)
	}
	return backups, nil
}

// GetBackup returns a backup by ID
func (s *DBBackupService) GetBackup(ctx context.Context, id int64) (*model.DBBackup, error) {
	var b model.DBBackup
	err := s.db.QueryRowContext(ctx,
		`SELECT id, db_server_id, db_version_id, database_id, database_name, backup_type, file_path, file_size, status, error_message, created_at
		FROM db_backups WHERE id = ?`, id).Scan(&b.ID, &b.DBServerID, &b.DBVersionID, &b.DatabaseID, &b.DatabaseName, &b.BackupType, &b.FilePath, &b.FileSize, &b.Status, &b.ErrorMessage, &b.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// DeleteBackup deletes a backup file and record
func (s *DBBackupService) DeleteBackup(ctx context.Context, id int64) error {
	backup, err := s.GetBackup(ctx, id)
	if err != nil {
		return fmt.Errorf("backup not found: %w", err)
	}

	// Delete file
	if err := os.Remove(backup.FilePath); err != nil && !os.IsNotExist(err) {
		log.Printf("failed to delete backup file %s: %v", backup.FilePath, err)
	}

	// Delete record
	_, err = s.db.ExecContext(ctx, "DELETE FROM db_backups WHERE id = ?", id)
	return err
}

// RestoreBackup restores a database from backup
func (s *DBBackupService) RestoreBackup(ctx context.Context, id int64, dbType string) error {
	backup, err := s.GetBackup(ctx, id)
	if err != nil {
		return fmt.Errorf("backup not found: %w", err)
	}

	if backup.Status != "completed" {
		return fmt.Errorf("backup is not in completed status")
	}

	// Check if file exists
	if _, err := os.Stat(backup.FilePath); os.IsNotExist(err) {
		return fmt.Errorf("backup file not found")
	}

	switch dbType {
	case "mysql":
		return s.restoreMySQL(ctx, backup)
	case "postgresql":
		return s.restorePostgreSQL(ctx, backup)
	case "redis":
		return s.restoreRedis(ctx, backup)
	default:
		return fmt.Errorf("unsupported db type: %s", dbType)
	}
}

// restoreMySQL restores a MySQL database from backup
func (s *DBBackupService) restoreMySQL(ctx context.Context, backup *model.DBBackup) error {
	out, _, err := s.executor.RunCombined(ctx, "bash", "-c", fmt.Sprintf("mysql %s < %s", backup.DatabaseName, backup.FilePath))
	if err != nil {
		return fmt.Errorf("mysql restore failed: %s", out)
	}
	return nil
}

// restorePostgreSQL restores a PostgreSQL database from backup
func (s *DBBackupService) restorePostgreSQL(ctx context.Context, backup *model.DBBackup) error {
	out, _, err := s.executor.RunCombined(ctx, "sudo", "-u", "postgres", "pg_restore", "-d", backup.DatabaseName, "-c", backup.FilePath)
	if err != nil {
		return fmt.Errorf("pg_restore failed: %s", out)
	}
	return nil
}

// restoreRedis restores a Redis database from backup
func (s *DBBackupService) restoreRedis(ctx context.Context, backup *model.DBBackup) error {
	// Stop Redis
	s.executor.RunCombined(ctx, "redis-cli", "SHUTDOWN", "NOSAVE")
	time.Sleep(1 * time.Second)

	// Copy RDB file
	data, err := os.ReadFile(backup.FilePath)
	if err != nil {
		return fmt.Errorf("read backup file: %w", err)
	}
	if err := os.WriteFile(DefaultRedisRDB, data, 0644); err != nil {
		return fmt.Errorf("write RDB file: %w", err)
	}

	// Start Redis
	_, _, err = s.executor.RunCombined(ctx, "systemctl", "start", "redis-server")
	if err != nil {
		return fmt.Errorf("start Redis failed: %w", err)
	}
	return nil
}

// CleanOldBackups removes old backups beyond the limit
func (s *DBBackupService) CleanOldBackups(ctx context.Context, databaseID int64, maxBackups int) error {
	if maxBackups <= 0 {
		maxBackups = MaxBackupsPerDB
	}

	// Get backups ordered by date
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, file_path FROM db_backups WHERE database_id = ? ORDER BY created_at DESC`, databaseID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var backups []struct {
		ID       int64
		FilePath string
	}
	for rows.Next() {
		var b struct {
			ID       int64
			FilePath string
		}
		if err := rows.Scan(&b.ID, &b.FilePath); err != nil {
			log.Printf("scan backup row for cleanup: %v", err)
			continue
		}
		backups = append(backups, b)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate backups for cleanup: %w", err)
	}

	// Delete old backups
	if len(backups) > maxBackups {
		for _, b := range backups[maxBackups:] {
			os.Remove(b.FilePath)
			s.db.ExecContext(ctx, "DELETE FROM db_backups WHERE id = ?", b.ID)
		}
	}

	return nil
}
