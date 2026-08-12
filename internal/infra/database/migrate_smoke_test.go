package database

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateFreshAppliesCronTimers(t *testing.T) {
	ctx := context.Background()
	migs := t.TempDir()
	copy := func(src, dst string) {
		b, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(migs, dst), b, 0644); err != nil {
			t.Fatal(err)
		}
	}
	// 从 repo 根复制两个迁移
	root := "../../../migrations"
	copy(filepath.Join(root, "000001_init_schema.up.sql"), "000001_init_schema.up.sql")
	copy(filepath.Join(root, "000002_cron_systemd_timers.up.sql"), "000002_cron_systemd_timers.up.sql")

	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := Migrate(db, migs); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// 两条迁移都应记录
	var n int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("期望 2 条迁移记录，实际 %d", n)
	}

	// cron_tasks / cron_logs 应已删除，scripts 无 content 列
	for _, tb := range []string{"cron_tasks", "cron_logs"} {
		var cnt int
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", tb).Scan(&cnt)
		if err != nil {
			t.Fatal(err)
		}
		if cnt != 0 {
			t.Errorf("表 %s 应已删除", tb)
		}
	}
	var contentCol int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM pragma_table_info('scripts') WHERE name='content'").Scan(&contentCol); err != nil {
		t.Fatal(err)
	}
	if contentCol != 0 {
		t.Errorf("scripts.content 列应已删除")
	}

	// 重启后幂等：再次 Migrate 不应报错、不应新增记录
	if err := Migrate(db, migs); err != nil {
		t.Fatalf("二次 migrate: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("二次迁移后记录数应为 2，实际 %d", n)
	}
}
