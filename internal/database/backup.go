package database

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"easyserver/internal/infra"
)

const (
	DefaultBackupDir = "/var/backups/easyserver/db"
	MaxBackupsPerDB  = 10
)

// SetBackupDir sets the backup directory.
func (s *Service) SetBackupDir(dir string) {
	s.backupDir = dir
}

func (s *Service) CreateBackup(ctx context.Context, instanceID int64, dbName string, dbType DBType) (*DBBackup, error) {
	if err := os.MkdirAll(s.backupDir, 0755); err != nil {
		return nil, fmt.Errorf("create backup dir: %w", err)
	}

	timestamp := time.Now().Format("20060102150405")
	var fileName string
	switch dbType {
	case DBTypeMySQL, DBTypePostgreSQL:
		fileName = fmt.Sprintf("%s_%s.sql", dbName, timestamp)
	case DBTypeRedis:
		fileName = fmt.Sprintf("dump_%s.rdb", timestamp)
	default:
		return nil, fmt.Errorf("unsupported db type: %s", dbType)
	}
	filePath := filepath.Join(s.backupDir, fileName)

	backup := &DBBackup{
		DBInstanceID: instanceID,
		DBType:       dbType,
		DatabaseName: dbName,
		BackupType:   "manual",
		FilePath:     filePath,
		Status:       "pending",
	}

	id, err := s.repo.CreateBackup(ctx, backup)
	if err != nil {
		return nil, err
	}
	backup.ID = id

	backupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	infra.Go(func() {
		defer cancel()
		s.executeBackup(backupCtx, backup, dbType)
	})

	return backup, nil
}

func (s *Service) executeBackup(ctx context.Context, backup *DBBackup, dbType DBType) {
	var err error

	switch dbType {
	case DBTypeMySQL:
		err = s.backupMySQL(ctx, backup)
	case DBTypePostgreSQL:
		err = s.backupPostgreSQL(ctx, backup)
	case DBTypeRedis:
		err = s.backupRedis(ctx, backup)
	}

	if err != nil {
		backup.Status = "failed"
		backup.ErrorMessage = err.Error()
		log.Printf("backup failed for %s: %v", backup.DatabaseName, err)
	} else {
		backup.Status = "completed"
		if info, err := os.Stat(backup.FilePath); err == nil {
			backup.FileSize = info.Size()
		}
	}

	if err := s.repo.UpdateBackupStatus(ctx, backup.ID, backup.Status, backup.FileSize, backup.ErrorMessage); err != nil {
		log.Printf("failed to update backup record %d: %v", backup.ID, err)
	}
}

func (s *Service) backupMySQL(ctx context.Context, backup *DBBackup) error {
	instance, err := s.repo.GetInstance(ctx, backup.DBInstanceID)
	if err != nil || instance == nil {
		return fmt.Errorf("database instance not found")
	}
	out, err := s.runInVersion(ctx, instance, "mysqldump", "--single-transaction", "--routines", "--triggers", backup.DatabaseName)
	if err != nil {
		return fmt.Errorf("mysqldump failed: %w", err)
	}
	return os.WriteFile(backup.FilePath, []byte(out), 0644)
}

func (s *Service) backupPostgreSQL(ctx context.Context, backup *DBBackup) error {
	instance, err := s.repo.GetInstance(ctx, backup.DBInstanceID)
	if err != nil || instance == nil {
		return fmt.Errorf("database instance not found")
	}
	out, err := s.runInVersion(ctx, instance, "pg_dump", "-Fc", backup.DatabaseName)
	if err != nil {
		return fmt.Errorf("pg_dump failed: %w", err)
	}
	return os.WriteFile(backup.FilePath, []byte(out), 0644)
}

func (s *Service) backupRedis(ctx context.Context, backup *DBBackup) error {
	instance, err := s.repo.GetInstance(ctx, backup.DBInstanceID)
	if err != nil || instance == nil {
		return fmt.Errorf("database instance not found")
	}
	// Trigger persistence over the direct connection; the dump file still has to
	// be copied out of the container (a container file operation).
	if err := s.redisFor().BgSave(ctx, instance); err != nil {
		return fmt.Errorf("redis BGSAVE failed: %w", err)
	}

	time.Sleep(2 * time.Second)
	return s.runtime.CopyFrom(ctx, instance.ContainerEngine, instance.ContainerName, "/data/dump.rdb", backup.FilePath)
}

func (s *Service) ListBackups(ctx context.Context, instanceID int64, dbName string) ([]DBBackup, error) {
	return s.repo.ListBackups(ctx, instanceID, dbName)
}

func (s *Service) GetBackup(ctx context.Context, id int64) (*DBBackup, error) {
	return s.repo.GetBackup(ctx, id)
}

func (s *Service) DeleteBackup(ctx context.Context, id int64) error {
	backup, err := s.repo.GetBackup(ctx, id)
	if err != nil {
		return fmt.Errorf("backup not found: %w", err)
	}

	if err := os.Remove(backup.FilePath); err != nil && !os.IsNotExist(err) {
		log.Printf("failed to delete backup file %s: %v", backup.FilePath, err)
	}

	return s.repo.DeleteBackup(ctx, id)
}

func (s *Service) RestoreBackup(ctx context.Context, id int64, dbType DBType) error {
	backup, err := s.repo.GetBackup(ctx, id)
	if err != nil {
		return fmt.Errorf("backup not found: %w", err)
	}

	if backup.Status != "completed" {
		return fmt.Errorf("backup is not in completed status")
	}

	if _, err := os.Stat(backup.FilePath); os.IsNotExist(err) {
		return fmt.Errorf("backup file not found")
	}

	switch dbType {
	case DBTypeMySQL:
		return s.restoreMySQL(ctx, backup)
	case DBTypePostgreSQL:
		return s.restorePostgreSQL(ctx, backup)
	case DBTypeRedis:
		return s.restoreRedis(ctx, backup)
	default:
		return fmt.Errorf("unsupported db type: %s", dbType)
	}
}

func (s *Service) restoreMySQL(ctx context.Context, backup *DBBackup) error {
	instance, err := s.repo.GetInstance(ctx, backup.DBInstanceID)
	if err != nil || instance == nil {
		return fmt.Errorf("database instance not found")
	}
	target := "/tmp/easyserver-restore.sql"
	if err := s.runtime.CopyTo(ctx, instance.ContainerEngine, instance.ContainerName, backup.FilePath, target); err != nil {
		return fmt.Errorf("copy backup into container: %w", err)
	}
	if _, err := s.runInVersion(ctx, instance, "sh", "-c", "mysql "+shellQuote(backup.DatabaseName)+" < "+target); err != nil {
		return fmt.Errorf("mysql restore failed: %w", err)
	}
	return nil
}

func (s *Service) restorePostgreSQL(ctx context.Context, backup *DBBackup) error {
	instance, err := s.repo.GetInstance(ctx, backup.DBInstanceID)
	if err != nil || instance == nil {
		return fmt.Errorf("database instance not found")
	}
	target := "/tmp/easyserver-restore.dump"
	if err := s.runtime.CopyTo(ctx, instance.ContainerEngine, instance.ContainerName, backup.FilePath, target); err != nil {
		return fmt.Errorf("copy backup into container: %w", err)
	}
	if _, err := s.runInVersion(ctx, instance, "pg_restore", "-d", backup.DatabaseName, "-c", target); err != nil {
		return fmt.Errorf("pg_restore failed: %s", SanitizeSQLError(err.Error()))
	}
	return nil
}

func (s *Service) restoreRedis(ctx context.Context, backup *DBBackup) error {
	instance, err := s.repo.GetInstance(ctx, backup.DBInstanceID)
	if err != nil || instance == nil {
		return fmt.Errorf("database instance not found")
	}
	if err := s.runtime.Stop(ctx, instance.ContainerEngine, instance.ContainerName); err != nil {
		return fmt.Errorf("stop Redis failed: %w", err)
	}
	if err := s.runtime.CopyTo(ctx, instance.ContainerEngine, instance.ContainerName, backup.FilePath, "/data/dump.rdb"); err != nil {
		return fmt.Errorf("copy Redis backup: %w", err)
	}
	if err := s.runtime.Start(ctx, instance.ContainerEngine, instance.ContainerName); err != nil {
		return fmt.Errorf("start Redis failed: %w", err)
	}
	return nil
}

func (s *Service) CleanOldBackups(ctx context.Context, instanceID int64, dbName string, maxBackups int) error {
	if maxBackups <= 0 {
		maxBackups = MaxBackupsPerDB
	}

	backups, err := s.repo.ListBackups(ctx, instanceID, dbName)
	if err != nil {
		return err
	}

	if len(backups) > maxBackups {
		for _, b := range backups[maxBackups:] {
			os.Remove(b.FilePath)
			s.repo.DeleteBackup(ctx, b.ID)
		}
	}

	return nil
}

// runInVersion runs a command inside the instance's container via the CLI
// runtime, with admin credentials injected. Used for engine-side tooling that
// has no driver equivalent (mysqldump / pg_dump / redis-cli) and config
// reloads — never for SQL data operations, which use the driver channel.
func (s *Service) runInVersion(ctx context.Context, instance *DBInstance, args ...string) (string, error) {
	if instance == nil || instance.ContainerEngine == "" || instance.ContainerName == "" {
		return "", fmt.Errorf("database instance is not container-managed")
	}
	args = s.withAdminCredentials(instance, args)
	return s.runtime.Exec(ctx, instance.ContainerEngine, instance.ContainerName, args...)
}

func (s *Service) withAdminCredentials(instance *DBInstance, args []string) []string {
	if len(args) == 0 || instance.AdminPassword == "" {
		return args
	}
	password := instance.AdminPassword
	switch instance.DBType {
	case DBTypeMySQL:
		// Password via MYSQL_PWD env instead of `-p` on the command line: mysql
		// prints "Using a password on the command line interface can be insecure."
		// to stderr on every `-p` invocation, and stderr is merged into the parsed
		// output (RunCombined), so the warning would surface as a bogus first row
		// in tabular listings. `exec -e` injects the env before the command.
		return append([]string{"-e", "MYSQL_PWD=" + password, args[0], "-uroot"}, args[1:]...)
	case DBTypePostgreSQL:
		return append([]string{args[0], "-U", "postgres"}, args[1:]...)
	}
	return args
}
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
