package plugin

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// openSourceTestDB 创建带 plugin_sources 表的测试库（表结构与迁移 000015 一致）。
func openSourceTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", t.TempDir()+"/test.db?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.ExecContext(context.Background(), `
		CREATE TABLE IF NOT EXISTS plugin_sources (
			id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT UNIQUE NOT NULL, url TEXT NOT NULL,
			enabled INTEGER DEFAULT 1, created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP)`,
	); err != nil {
		t.Fatal(err)
	}
	return db
}

// sourceRepo 的 CRUD 与校验测试（表 plugin_sources 由迁移 000015 创建）。

func TestSourceRepositoryCRUD(t *testing.T) {
	db := openSourceTestDB(t)
	ctx := context.Background()
	repo := NewSourceRepository(db)

	// 初始为空
	list, err := repo.ListSources(ctx)
	if err != nil {
		t.Fatalf("ListSources: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("初始应无插件源，got %d", len(list))
	}

	// 添加
	src, err := repo.AddSource(ctx, "official", "https://plugins.example.com")
	if err != nil {
		t.Fatalf("AddSource: %v", err)
	}
	if src.ID == 0 || !src.Enabled {
		t.Fatalf("AddSource 返回异常: %+v", src)
	}

	// 重复添加同名应报错
	if _, err := repo.AddSource(ctx, "official", "https://plugins.example.com"); err == nil {
		t.Fatal("重复添加同名插件源应报错")
	}

	// 列表应含 1 条
	list, err = repo.ListSources(ctx)
	if err != nil {
		t.Fatalf("ListSources: %v", err)
	}
	if len(list) != 1 || list[0].Name != "official" || list[0].URL != "https://plugins.example.com" {
		t.Fatalf("ListSources 结果异常: %+v", list)
	}

	// 删除
	if err := repo.DeleteSource(ctx, src.ID); err != nil {
		t.Fatalf("DeleteSource: %v", err)
	}
	if err := repo.DeleteSource(ctx, src.ID); err == nil {
		t.Fatal("删除不存在的插件源应报错")
	}
	list, _ = repo.ListSources(ctx)
	if len(list) != 0 {
		t.Fatalf("删除后应无插件源，got %d", len(list))
	}
}

func TestSourceRepositorySetEnabled(t *testing.T) {
	db := openSourceTestDB(t)
	ctx := context.Background()
	repo := NewSourceRepository(db)

	src, err := repo.AddSource(ctx, "official", "https://plugins.example.com")
	if err != nil {
		t.Fatalf("AddSource: %v", err)
	}
	if err := repo.SetSourceEnabled(ctx, src.ID, false); err != nil {
		t.Fatalf("SetSourceEnabled: %v", err)
	}
	list, _ := repo.ListSources(ctx)
	if len(list) != 1 || list[0].Enabled {
		t.Fatalf("SetSourceEnabled(false) 后应禁用: %+v", list)
	}
	if err := repo.SetSourceEnabled(ctx, src.ID+999, false); err == nil {
		t.Fatal("对不存在的源 SetSourceEnabled 应报错")
	}
}

func TestSourceAddValidation(t *testing.T) {
	db := openSourceTestDB(t)
	mgr := &Manager{}
	svc := NewSourceService(NewSourceRepository(db), mgr)

	if _, err := svc.Add(context.Background(), "", "https://x.com"); err == nil {
		t.Error("空名应被拒绝")
	}
	if _, err := svc.Add(context.Background(), "bad name!", "https://x.com"); err == nil {
		t.Error("非法名应被拒绝")
	}
	if _, err := svc.Add(context.Background(), "ok", "ftp://x.com"); err == nil {
		t.Error("非 http(s) 地址应被拒绝")
	}
	if _, err := svc.Add(context.Background(), "ok", "not-a-url"); err == nil {
		t.Error("非法 URL 应被拒绝")
	}
}
