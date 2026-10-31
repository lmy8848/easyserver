package plugin

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func testRepo(t *testing.T) (PluginRepository, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite", t.TempDir()+"/test.db?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1) // 测试里有健康检查/退避并发写，单连接避免 SQLITE_BUSY
	t.Cleanup(func() { _ = db.Close() })
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS plugins (
			id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT UNIQUE NOT NULL, version TEXT NOT NULL,
			api_version INTEGER NOT NULL, manifest_path TEXT NOT NULL, enabled INTEGER DEFAULT 0,
			status TEXT DEFAULT 'registered', pid INTEGER DEFAULT 0, permissions_approved TEXT,
			restart_count INTEGER DEFAULT 0, last_error TEXT, last_seen DATETIME, socket_path TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE IF NOT EXISTS plugin_config (
			plugin_name TEXT NOT NULL, key TEXT NOT NULL, value TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY (plugin_name, key))`,
		`CREATE TABLE IF NOT EXISTS plugin_secrets (
			plugin_name TEXT NOT NULL, key TEXT NOT NULL, encrypted_value TEXT NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY (plugin_name, key))`,
	} {
		if _, err := db.ExecContext(context.Background(), q); err != nil {
			t.Fatal(err)
		}
	}
	repo, err := NewSQLiteRepository(db, "test-root-key", "")
	if err != nil {
		t.Fatal(err)
	}
	return repo, db
}

func TestPluginRecordRoundtrip(t *testing.T) {
	ctx := context.Background()
	repo, _ := testRepo(t)

	rec := &PluginRecord{
		Name: "demo", Version: "0.1.0", APIVersion: 1, ManifestPath: "/opt/easyserver/plugins/demo/plugin.yaml",
		Enabled: true, Status: StateRunning, PID: 1234,
		PermissionsApproved: []string{PermAuditWrite, PermExecRestricted},
		SocketPath:          "/run/easyserver/plugins/demo.sock",
	}
	if err := repo.UpsertPlugin(ctx, rec); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetPlugin(ctx, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Status != StateRunning || len(got.PermissionsApproved) != 2 {
		t.Fatalf("读回不一致: %+v", got)
	}
	// PID 由运行时经 SetPluginStatus 管理
	if err := repo.SetPluginStatus(ctx, "demo", StateRunning, 1234, ""); err != nil {
		t.Fatal(err)
	}
	if got, _ = repo.GetPlugin(ctx, "demo"); got.PID != 1234 {
		t.Fatalf("PID 未落库: %+v", got)
	}

	// Upsert 幂等：同 name 更新而非新增
	rec.Version = "0.2.0"
	if err := repo.UpsertPlugin(ctx, rec); err != nil {
		t.Fatal(err)
	}
	list, err := repo.ListPlugins(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Version != "0.2.0" {
		t.Fatalf("Upsert 应为更新语义: %+v", list)
	}

	if err := repo.SetPluginStatus(ctx, "demo", StateDegraded, 0, "boom"); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.GetPlugin(ctx, "demo")
	if got.Status != StateDegraded || got.LastError != "boom" {
		t.Fatalf("状态更新未生效: %+v", got)
	}

	if err := repo.SetPluginEnabled(ctx, "demo", false, []string{PermAuditWrite}); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.GetPlugin(ctx, "demo")
	if got.Enabled || len(got.PermissionsApproved) != 1 {
		t.Fatalf("启停更新未生效: %+v", got)
	}

	if err := repo.DeletePlugin(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.GetPlugin(ctx, "demo"); got != nil {
		t.Fatal("删除后不应存在")
	}
}

func TestPluginConfigKV(t *testing.T) {
	ctx := context.Background()
	repo, _ := testRepo(t)
	if err := repo.SetConfig(ctx, "demo", "interval", "30s"); err != nil {
		t.Fatal(err)
	}
	if v, _ := repo.GetConfig(ctx, "demo", "interval"); v != "30s" {
		t.Fatalf("读回不一致: %q", v)
	}
	if err := repo.SetConfig(ctx, "demo", "interval", "60s"); err != nil {
		t.Fatal(err)
	}
	all, _ := repo.ListConfig(ctx, "demo")
	if len(all) != 1 || all["interval"] != "60s" {
		t.Fatalf("覆盖写入异常: %+v", all)
	}
	if _, err := repo.GetConfig(ctx, "demo", "missing"); err == nil {
		t.Fatal("不存在的配置应报错")
	}
	_ = repo.DeleteConfig(ctx, "demo", "interval")
	if _, err := repo.GetConfig(ctx, "demo", "interval"); err == nil {
		t.Fatal("删除后应报错")
	}
}

func TestPluginSecretsEncrypted(t *testing.T) {
	ctx := context.Background()
	repo, db := testRepo(t)

	if err := repo.SetSecret(ctx, "demo", "api_key", "super-secret-value"); err != nil {
		t.Fatal(err)
	}
	if v, _ := repo.GetSecret(ctx, "demo", "api_key"); v != "super-secret-value" {
		t.Fatalf("解密读回不一致: %q", v)
	}

	// 落库的是 AES-GCM 密文（base64），不应包含明文
	var enc string
	if err := db.QueryRowContext(ctx,
		"SELECT encrypted_value FROM plugin_secrets WHERE plugin_name = 'demo' AND key = 'api_key'").Scan(&enc); err != nil {
		t.Fatal(err)
	}
	if enc == "" || strings.Contains(enc, "super-secret-value") {
		t.Fatalf("密钥应加密存储: %q", enc)
	}

	// 不同插件命名空间隔离
	if err := repo.SetSecret(ctx, "other", "api_key", "other-value"); err != nil {
		t.Fatal(err)
	}
	if v, _ := repo.GetSecret(ctx, "demo", "api_key"); v != "super-secret-value" {
		t.Fatal("命名空间串扰")
	}
	_ = repo.DeleteSecret(ctx, "demo", "api_key")
	if _, err := repo.GetSecret(ctx, "demo", "api_key"); err == nil {
		t.Fatal("删除后应报错")
	}
}
