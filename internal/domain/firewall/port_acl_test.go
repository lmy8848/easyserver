package firewall

import (
	"context"
	"database/sql"
	"reflect"
	"testing"

	_ "modernc.org/sqlite"
)

func testACLRepo(t *testing.T) PortACLRepository {
	t.Helper()
	db, err := sql.Open("sqlite", t.TempDir()+"/acl.db")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.ExecContext(context.Background(), `CREATE TABLE IF NOT EXISTS port_acl (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		port INTEGER NOT NULL, protocol TEXT NOT NULL DEFAULT 'tcp',
		cidrs TEXT NOT NULL, remark TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(port, protocol))`); err != nil {
		t.Fatal(err)
	}
	return NewPortACLSQLiteRepository(db)
}

func TestNormalizeCIDRs(t *testing.T) {
	got, err := normalizeCIDRs([]string{" 1.2.3.4 ", "10.0.0.0/8", "1.2.3.4", ""})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"1.2.3.4/32", "10.0.0.0/8"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("normalize 结果不符: got %v want %v", got, want)
	}
	if _, err := normalizeCIDRs([]string{"not-an-ip"}); err == nil {
		t.Error("非法 IP 应报错")
	}
	if _, err := normalizeCIDRs(nil); err == nil {
		t.Error("空列表应报错")
	}
	if _, err := normalizeCIDRs([]string{"", "  "}); err == nil {
		t.Error("全空白列表应报错")
	}
}

func TestPortACLRepoRoundtrip(t *testing.T) {
	repo := testACLRepo(t)
	ctx := context.Background()

	acl := &PortACL{Port: 43210, Protocol: "tcp", CIDRs: []string{"203.0.113.0/24", "1.2.3.4/32"}, Remark: "test"}
	if err := repo.Upsert(ctx, acl); err != nil {
		t.Fatal(err)
	}
	if acl.ID == 0 {
		t.Fatal("upsert 应回填 ID")
	}

	got, err := repo.Get(ctx, 43210, "tcp")
	if err != nil || got == nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Remark != "test" || len(got.CIDRs) != 2 {
		t.Errorf("Get 结果不符: %+v", got)
	}

	// 覆盖更新（按 port+protocol 幂等）
	acl.CIDRs = []string{"10.0.0.0/8"}
	if err := repo.Upsert(ctx, acl); err != nil {
		t.Fatal(err)
	}
	got2, _ := repo.Get(ctx, 43210, "tcp")
	if len(got2.CIDRs) != 1 || got2.CIDRs[0] != "10.0.0.0/8" {
		t.Errorf("更新未生效: %+v", got2)
	}

	// List
	all, err := repo.List(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("List: %v len=%d", err, len(all))
	}

	// Delete
	if err := repo.Delete(ctx, 43210, "tcp"); err != nil {
		t.Fatal(err)
	}
	got3, _ := repo.Get(ctx, 43210, "tcp")
	if got3 != nil {
		t.Error("删除后应查不到")
	}
}
