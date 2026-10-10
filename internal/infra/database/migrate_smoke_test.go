package database

import (
	"context"
	"database/sql"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// embeddedUpCount 统计嵌入迁移链中的 .up.sql 文件数，作为迁移记录数基准。
func embeddedUpCount(t *testing.T) int {
	t.Helper()
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := fs.ReadDir(sub, ".")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			n++
		}
	}
	return n
}

func TestMigrateFreshAppliesEmbeddedSchema(t *testing.T) {
	ctx := context.Background()

	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// 全部嵌入迁移都应记录
	want := embeddedUpCount(t)
	var n int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != want {
		t.Fatalf("期望 %d 条迁移记录，实际 %d", want, n)
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
	if err := Migrate(db); err != nil {
		t.Fatalf("二次 migrate: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != want {
		t.Fatalf("二次迁移后记录数应为 %d，实际 %d", want, n)
	}
}
