package database

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"easyserver/internal/infra/task"
)

func (s *Service) CreateBackup(ctx context.Context, instanceID int64, dbName string, dbType DBType) (*DBBackup, error) {
	instance, err := s.repo.GetInstance(ctx, instanceID)
	if err != nil || instance == nil {
		return nil, fmt.Errorf("database instance not found")
	}
	// 备份直接落在实例宿主数据目录的 es_backups/ 子目录 —— 该目录是宿主挂载，
	// 容器内 dump 写这里宿主直见，无需 CopyFrom 往返。chown 999 让容器内进程
	// （pg_dump 等以 uid 999 运行）能写入。
	backupDir := filepath.Join(instance.VolumeName, "es_backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return nil, fmt.Errorf("create backup dir: %w", err)
	}
	// chown 999 让容器内进程（pg_dump 等以 uid 999 运行）能写入；非 root 环境跳过
	// （与 prepareHostDirs 一致，rootless/测试 chown 会 EPERM）。
	if os.Geteuid() == 0 {
		if err := os.Chown(backupDir, containerUID, containerUID); err != nil {
			return nil, fmt.Errorf("chown backup dir: %w", err)
		}
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
	filePath := filepath.Join(backupDir, fileName)

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

	// 备份在内存任务里执行。key 按 (instance, db) 固定 —— taskMgr 同 key 去重，
	// 天然保证同一个库同一时间只有一个备份在跑；重复点击会收到 ErrKeyBusy。
	key := fmt.Sprintf("backup-%d-%s", instanceID, dbName)
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
	// 容器内写文件（-r），路径映射到宿主数据目录 es_backups/（宿主挂载 → 宿主直见），
	// 不再走 docker cp 往返。
	target := containerDataDir(instance) + "/es_backups/" + filepath.Base(backup.FilePath)
	// --set-gtid-purged=OFF：不写 SET @@GLOBAL.GTID_PURGED。GTID 模式下 mysqldump
	// 默认(AUTO/ON)会输出该语句，恢复到已有 GTID 的实例报 3546（GTID set 不允许
	// 与已执行 GTID 重叠/变更）。本面板的恢复目标是同一/另一实例的现有库，
	// OFF 让备份不携带 GTID，恢复不碰 @@GLOBAL。
	if _, err := s.runInContainer(ctx, instance, "mysqldump", "-r", target, "--single-transaction", "--set-gtid-purged=OFF", "--routines", "--triggers", backup.DatabaseName); err != nil {
		return fmt.Errorf("mysqldump failed: %w", err)
	}
	return nil
}

func (s *Service) backupPostgreSQL(ctx context.Context, backup *DBBackup) error {
	instance, err := s.repo.GetInstance(ctx, backup.DBInstanceID)
	if err != nil || instance == nil {
		return fmt.Errorf("database instance not found")
	}
	// 同上：pg_dump -Fc 是二进制，必须 -f 写容器文件 —— 但目标在宿主挂载的数据
	// 目录内，写完宿主直见，无需 cp。
	target := containerDataDir(instance) + "/es_backups/" + filepath.Base(backup.FilePath)
	if _, err := s.runInContainer(ctx, instance, "pg_dump", "-Fc", "-f", target, backup.DatabaseName); err != nil {
		return fmt.Errorf("pg_dump failed: %w", err)
	}
	return nil
}

func (s *Service) backupRedis(ctx context.Context, backup *DBBackup) error {
	instance, err := s.repo.GetInstance(ctx, backup.DBInstanceID)
	if err != nil || instance == nil {
		return fmt.Errorf("database instance not found")
	}
	// Trigger persistence over the direct connection. dump.rdb 由 redis 进程直接写在
	// 宿主数据目录（/data 是宿主挂载），宿主侧拷贝到 es_backups/ 即可，不经过容器
	// 文件通道。
	if err := s.redisFor().BgSave(ctx, instance); err != nil {
		return fmt.Errorf("redis BGSAVE failed: %w", err)
	}

	time.Sleep(2 * time.Second)
	src := filepath.Join(instance.VolumeName, "dump.rdb")
	if err := copyFile(src, backup.FilePath); err != nil {
		return fmt.Errorf("copy redis dump: %w", err)
	}
	return nil
}

// copyFile copies a file byte-for-byte (host-side file operation).
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
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

	// 恢复任务若正跑该备份，删文件会让恢复失败——拒绝删除。restoreTask 保留最近
	// 一次恢复的终态（供 SSE 查询），所以只拦 running，终态（含失败）可删。
	s.restoreMu.Lock()
	st, restoring := s.restoreTask[id]
	s.restoreMu.Unlock()
	if restoring && st.Status == "running" {
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
	// `--execute="source ..."` 会把 source 当 SQL 语句解析（1064）——source 是客户端
	// 命令。改为容器内 sh 重定向：mysql db < 文件 从 stdin 喂 SQL。这里不走
	// runInContainer：withAdminCredentials 会给任意命令追加 -uroot，sh 不认该选项
	// 报 invalid option name；手动构造 `-e MYSQL_PWD=…`（execCommand 提升为容器
	// exec 环境）+ `sh -c`，凭据不进命令行。库名经 isValidDBName 白名单。
	cmd := fmt.Sprintf("mysql -uroot %s < %s", backup.DatabaseName, target)
	args := []string{"-e", "MYSQL_PWD=" + instance.AdminPassword, "sh", "-c", cmd}
	if _, err := s.runtime.Exec(ctx, instance.ContainerEngine, instance.ContainerName, args...); err != nil {
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
	if _, err := s.runInContainer(ctx, instance, "pg_restore", "--single-transaction", "--if-exists", "-c", "-d", backup.DatabaseName, target); err != nil {
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

// WaitBackup 返回某备份任务的完成信号（backupID → task Done channel）。
// 成功完成的任务会被 taskMgr 即清（Get 返回 !ok），此时返回已关闭的 channel，
// 调用方立即去读 DB 行的终态；running 任务返回其 Done，任务结束即关闭。
// 供 SSE 状态流使用，替代轮询 DB 行。
func (s *Service) WaitBackup(backupID int64) (<-chan struct{}, error) {
	backup, err := s.repo.GetBackup(context.Background(), backupID)
	if err != nil {
		return nil, err
	}
	key := fmt.Sprintf("backup-%d-%s", backup.DBInstanceID, backup.DatabaseName)
	tk, ok := s.taskMgr.Get(key)
	if !ok {
		done := make(chan struct{})
		close(done)
		return done, nil
	}
	return tk.Done(), nil
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
