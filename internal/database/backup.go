package database

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"easyserver/internal/infra/task"
)

const (
	DefaultBackupDir = "/opt/easyserver/backups/db"
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
	// 容器内写文件（-r），再 docker cp 出来——避免 dump 走 stdout 字符串管道
	// （大库截断/二进制损坏风险，见 runInContainer 注释）。
	target := "/tmp/easyserver-backup.sql"
	if _, err := s.runInContainer(ctx, instance, "mysqldump", "-r", target, "--single-transaction", "--routines", "--triggers", backup.DatabaseName); err != nil {
		return fmt.Errorf("mysqldump failed: %w", err)
	}
	return s.runtime.CopyFrom(ctx, instance.ContainerEngine, instance.ContainerName, target, backup.FilePath)
}

func (s *Service) backupPostgreSQL(ctx context.Context, backup *DBBackup) error {
	instance, err := s.repo.GetInstance(ctx, backup.DBInstanceID)
	if err != nil || instance == nil {
		return fmt.Errorf("database instance not found")
	}
	// 同上：pg_dump -Fc 是二进制，必须 -f 写容器文件再 cp 出来。
	target := "/tmp/easyserver-backup.dump"
	if _, err := s.runInContainer(ctx, instance, "pg_dump", "-Fc", "-f", target, backup.DatabaseName); err != nil {
		return fmt.Errorf("pg_dump failed: %w", err)
	}
	return s.runtime.CopyFrom(ctx, instance.ContainerEngine, instance.ContainerName, target, backup.FilePath)
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

	// 备份任务运行中不能删（task 可能正在写该行）。
	if backup.Status == "running" {
		return fmt.Errorf("备份进行中，请等待完成后再删除")
	}

	if err := os.Remove(backup.FilePath); err != nil && !os.IsNotExist(err) {
		log.Printf("failed to delete backup file %s: %v", backup.FilePath, err)
	}

	// 恢复任务若正跑该备份，删文件会让恢复失败——拒绝删除。
	s.restoreMu.Lock()
	_, restoring := s.restoreTask[id]
	s.restoreMu.Unlock()
	if restoring {
		return fmt.Errorf("该备份正在恢复中，无法删除")
	}

	return s.repo.DeleteBackup(ctx, id)
}

// RestoreBackup 在内存任务里异步恢复。恢复是纯内存操作：状态只存 restoreTask map
// （GET /backups/:bid/restore-status 以 SSE 推送），不碰备份行的 status 列——恢复失败不
// 表示备份坏了。无超时（大库慢盘不受时限）；进程崩溃后内存态丢失，SSE 重连收到
// done 帧提示，用户需手动确认，这是内存态的诚实语义。
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

	key := fmt.Sprintf("restore-%d", id)
	st := &RestoreStatus{Status: "running", StartedAt: time.Now().Format("2006-01-02 15:04:05")}
	s.restoreMu.Lock()
	s.restoreTask[id] = st
	s.restoreMu.Unlock()

	if _, err := s.taskMgr.Start(key, task.Options{}, func(ctx context.Context) error {
		var rerr error
		switch dbType {
		case DBTypeMySQL:
			rerr = s.restoreMySQL(ctx, backup)
		case DBTypePostgreSQL:
			rerr = s.restorePostgreSQL(ctx, backup)
		case DBTypeRedis:
			rerr = s.restoreRedis(ctx, backup)
		default:
			rerr = fmt.Errorf("unsupported db type: %s", dbType)
		}

		s.restoreMu.Lock()
		if rerr != nil {
			st.Status = "failed"
			st.Error = rerr.Error()
			log.Printf("restore failed for backup %d: %v", backup.ID, rerr)
		} else {
			st.Status = "success"
		}
		s.restoreMu.Unlock()
		return rerr
	}); err != nil {
		// 启动失败（去重/并发满）：清掉内存态，回滚到"无恢复"。
		s.restoreMu.Lock()
		delete(s.restoreTask, id)
		s.restoreMu.Unlock()
		return fmt.Errorf("启动恢复任务失败: %w", err)
	}
	return nil
}

// GetRestoreStatus 返回某备份的恢复任务内存态；无进行中/最近的恢复返回 ok=false。
func (s *Service) GetRestoreStatus(_ context.Context, id int64) (*RestoreStatus, bool) {
	s.restoreMu.Lock()
	defer s.restoreMu.Unlock()
	st, ok := s.restoreTask[id]
	return st, ok
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
	// mysql 客户端执行文件：库名走位置参数（--execute 分支），不拼 shell —— 无注入面。
	if _, err := s.runInContainer(ctx, instance, "mysql", "--execute=source "+target, backup.DatabaseName); err != nil {
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
	// --single-transaction：整个恢复包在一个事务，中途失败自动 ROLLBACK，
	// 数据库回到恢复前状态——消除"半恢复"。
	if _, err := s.runInContainer(ctx, instance, "pg_restore", "--single-transaction", "-d", backup.DatabaseName, "-c", target); err != nil {
		return fmt.Errorf("pg_restore failed: %s", SanitizeSQLError(err.Error()))
	}
	return nil
}

func (s *Service) restoreRedis(ctx context.Context, backup *DBBackup) error {
	instance, err := s.repo.GetInstance(ctx, backup.DBInstanceID)
	if err != nil || instance == nil {
		return fmt.Errorf("database instance not found")
	}
	// AOF 开启时 Redis 启动忽略 RDB（AOF 优先），恢复 RDB 会静默失效——直接拒绝。
	if aof, err := s.redisFor().ConfigGet(ctx, instance, "appendonly"); err == nil && aof == "yes" {
		return fmt.Errorf("Redis 已开启 AOF（appendonly=yes），RDB 恢复会被忽略；请先在配置中关闭 AOF 再恢复")
	}
	// 覆盖前先留底原 dump.rdb，恢复失败能回滚。
	rollback := backup.FilePath + ".pre-restore"
	if err := s.runtime.CopyFrom(ctx, instance.ContainerEngine, instance.ContainerName, "/data/dump.rdb", rollback); err != nil {
		log.Printf("backup existing dump.rdb for rollback failed (continuing): %v", err)
	}
	if err := s.runtime.Stop(ctx, instance.ContainerEngine, instance.ContainerName); err != nil {
		return fmt.Errorf("stop Redis failed: %w", err)
	}
	if err := s.runtime.CopyTo(ctx, instance.ContainerEngine, instance.ContainerName, backup.FilePath, "/data/dump.rdb"); err != nil {
		_ = s.runtime.Start(ctx, instance.ContainerEngine, instance.ContainerName)
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

// runInContainer runs a command inside the instance's container via the CLI
// runtime, with admin credentials injected. Used for engine-side tooling that
// has no driver equivalent (mysqldump / pg_dump / redis-cli) and config
// reloads — never for SQL data operations, which use the driver channel.
func (s *Service) runInContainer(ctx context.Context, instance *DBInstance, args ...string) (string, error) {
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
