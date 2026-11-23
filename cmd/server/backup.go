package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	backuphttp "easyserver/internal/domain/backup/http"
	"easyserver/internal/infra/config"
	"easyserver/internal/infra/database"
)

// backupCmd 生成一份面板备份（配置 + SQLite 一致性快照），供 systemd timer 定时调用。
//
// 与 HTTP 导出共用 backuphttp.BuildArchive，区别只是不走鉴权、直接落到本地目录，
// 因此可在没有登录会话的情况下由运维定时任务执行。
func backupCmd(configPath string, args []string) {
	fs := flag.NewFlagSet("backup", flag.ExitOnError)
	out := fs.String("out", filepath.Join(config.DataRoot, "backups"), "输出目录")
	include := fs.String("include", "config,db", "包含项：config,db,sites,plugins")
	keepDays := fs.Int("keep-days", 14, "自动备份保留天数（0=不清理）")
	_ = fs.Parse(args)

	if _, err := config.Load(configPath); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	db, err := database.Init()
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// 配置仓库只为读 Path；出错不影响数据库快照。
	cfg, _ := config.Load(configPath)
	store := config.NewStore(cfg)

	if err := os.MkdirAll(*out, 0o700); err != nil {
		log.Fatalf("创建输出目录失败: %v", err)
	}
	ts := time.Now().Format("20060102-150405")
	finalPath := filepath.Join(*out, "auto-backup-"+ts+".tar.gz")
	tmpPath := finalPath + ".part"

	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		log.Fatalf("创建备份文件失败: %v", err)
	}
	includes := parseIncludeFlag(*include)
	warnings, berr := backuphttp.BuildArchive(context.Background(), store, db, f, os.TempDir(), includes)
	cerr := f.Close()
	if berr != nil {
		_ = os.Remove(tmpPath)
		log.Fatalf("打包失败: %v", berr)
	}
	if cerr != nil {
		_ = os.Remove(tmpPath)
		log.Fatalf("写入备份失败: %v", cerr)
	}
	// 先写 .part 再改名：中途失败不会留下"看起来正常"的截断包。
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		log.Fatalf("落盘备份失败: %v", err)
	}
	for _, w := range warnings {
		log.Printf("WARNING: %s", w)
	}
	fmt.Println(finalPath)

	if *keepDays > 0 {
		pruneBackups(*out, *keepDays)
	}
}

func parseIncludeFlag(raw string) map[string]bool {
	out := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		switch strings.TrimSpace(part) {
		case "config", "db", "sites", "plugins":
			out[strings.TrimSpace(part)] = true
		}
	}
	if len(out) == 0 {
		return map[string]bool{"config": true, "db": true}
	}
	return out
}

// pruneBackups 删除超过保留天数的自动备份（手工导出的备份不动）。
func pruneBackups(dir string, keepDays int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Printf("WARNING: 读取备份目录失败，跳过清理: %v", err)
		return
	}
	cutoff := time.Now().AddDate(0, 0, -keepDays)
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "auto-backup-") {
			continue
		}
		info, ierr := e.Info()
		if ierr != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		if rerr := os.Remove(filepath.Join(dir, e.Name())); rerr != nil {
			log.Printf("WARNING: 删除旧备份 %s 失败: %v", e.Name(), rerr)
		}
	}
}
