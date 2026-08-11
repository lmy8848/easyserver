package database

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"easyserver/internal/infra/task"
)

const (
	DefaultBackupDir = "/var/backups/easyserver/db"
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
		Status:       "running",
	}

	id, err := s.repo.CreateBackup(ctx, backup)
	if err != nil {
		return nil, err
	}
	backup.ID = id

	// 备份在内存任务里执行（taskMgr 去重 + 并发 + panic 恢复，无超时——大库慢
	// 盘不受时限）。DB 行状态是内存任务的镜像：任务结束写终态，进程崩溃则靠
	// 启动清扫把 running 标 failed。
	key := fmt.Sprintf("backup-%d", id)
	if _, err := s.taskMgr.Start(key, task.Options{}, func(ctx context.Context) error {
		return s.executeBackup(ctx, backup, dbType)
	}); err != nil {
		_ = s.repo.UpdateBackupStatus(ctx, id, "failed", 0, err.Error())
		return nil, err
	}
	return backup, nil
}

func (s *Service) executeBackup(ctx context.Context, backup *DBBackup, dbType DBType) error {
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
		backup.Status = "success"
		if info, err := os.Stat(backup.FilePath); err == nil {
			backup.FileSize = info.Size()
		}
	}

	if err := s.repo.UpdateBackupStatus(ctx, backup.ID, backup.Status, backup.FileSize, backup.ErrorMessage); err != nil {
		log.Printf("failed to update backup record %d: %v", backup.ID, err)
	}
	return err
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

	// 运行中的备份/恢复不能删：task 可能正在写该行或读该文件。
	if backup.Status == "running" {
		return fmt.Errorf("备份/恢复进行中，请等待完成后再删除")
	}

	if err := os.Remove(backup.FilePath); err != nil && !os.IsNotExist(err) {
		log.Printf("failed to delete backup file %s: %v", backup.FilePath, err)
	}

	return s.repo.DeleteBackup(ctx, id)
}

// RestoreBackup 在内存任务里异步恢复：DB 行置 running，恢复成功/失败后写回终态。
// 无超时（大库慢盘不受时限）；进程崩溃后该行由启动清扫标 failed。
func (s *Service) RestoreBackup(ctx context.Context, id int64, dbType DBType) error {
	backup, err := s.repo.GetBackup(ctx, id)
	if err != nil {
		return fmt.Errorf("backup not found: %w", err)
	}

	if backup.Status != "success" {
		return fmt.Errorf("备份不是已完成状态，无法恢复")
	}

	if _, err := os.Stat(backup.FilePath); os.IsNotExist(err) {
		return fmt.Errorf("备份文件不存在")
	}

	// DB 行先置 running（任务还没起，先落状态防重复点按），再启动内存任务。
	// 任务结束写终态；进程崩溃则靠启动清扫把 running 标 failed。
	if err := s.repo.UpdateBackupStatus(ctx, backup.ID, "running", backup.FileSize, ""); err != nil {
		return fmt.Errorf("标记恢复中失败: %w", err)
	}

	key := fmt.Sprintf("restore-%d", id)
	if _, err := s.taskMgr.Start(key, task.Options{}, func(ctx context.Context) error {
		var err error
		switch dbType {
		case DBTypeMySQL:
			err = s.restoreMySQL(ctx, backup)
		case DBTypePostgreSQL:
			err = s.restorePostgreSQL(ctx, backup)
		case DBTypeRedis:
			err = s.restoreRedis(ctx, backup)
		default:
			err = fmt.Errorf("unsupported db type: %s", dbType)
		}

		status := "success"
		errMsg := ""
		if err != nil {
			status = "failed"
			errMsg = err.Error()
			log.Printf("restore failed for backup %d: %v", backup.ID, err)
		}
		if uerr := s.repo.UpdateBackupStatus(context.Background(), backup.ID, status, backup.FileSize, errMsg); uerr != nil {
			log.Printf("failed to update backup record %d: %v", backup.ID, uerr)
		}
		return err
	}); err != nil {
		return fmt.Errorf("启动恢复任务失败: %w", err)
	}
	return nil
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

// SweepOrphanBackups 把 DB 里卡在 running 的备份行收敛为 failed —— 这些行的内存
// 任务已随进程崩溃丢失（task manager 无持久态），不收敛会永远显示"进行中"。
// 启动时调用一次。
func (s *Service) SweepOrphanBackups(ctx context.Context) {
	rows, err := s.repo.ListAllBackups(ctx)
	if err != nil {
		log.Printf("sweep orphan backups: %v", err)
		return
	}
	for _, b := range rows {
		if b.Status == "running" {
			_ = s.repo.UpdateBackupStatus(ctx, b.ID, "failed", 0, "服务重启中断")
			log.Printf("swept orphan backup %d (running → failed)", b.ID)
		}
	}
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
