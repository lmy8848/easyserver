package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupAuditTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	// :memory: gives each connection its own DB; pin to one connection.
	db.SetMaxOpenConns(1)
	queries := []string{
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER DEFAULT 0,
			username TEXT NOT NULL,
			action TEXT NOT NULL,
			resource TEXT DEFAULT '',
			detail TEXT DEFAULT '',
			ip TEXT DEFAULT '',
			user_agent TEXT DEFAULT '',
			type TEXT NOT NULL DEFAULT 'operation',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}
	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func newTestAuditService(db *sql.DB) *Service {
	repo := NewSQLiteRepository(db)
	svc := NewService(db, repo, 90)
	return svc
}

// --- TestLogOperation ---

func TestLogOperation(t *testing.T) {
	db := setupAuditTestDB(t)
	defer db.Close()
	svc := newTestAuditService(db)

	svc.LogOperation(context.Background(), 1, "admin", "TEST_ACTION", "/test", map[string]interface{}{"detail": "test detail"}, "127.0.0.1", "test-agent")
	svc.Close() // drain and flush to DB

	var userID int64
	var username, action, ip, detail string
	err := db.QueryRow("SELECT user_id, username, action, ip, detail FROM audit_logs WHERE action = 'TEST_ACTION'").
		Scan(&userID, &username, &action, &ip, &detail)
	if err != nil {
		t.Fatalf("query audit_logs: %v", err)
	}
	if userID != 1 {
		t.Errorf("userID = %d, want 1", userID)
	}
	if username != "admin" {
		t.Errorf("username = %q, want %q", username, "admin")
	}
	if action != "TEST_ACTION" {
		t.Errorf("action = %q, want %q", action, "TEST_ACTION")
	}
	if ip != "127.0.0.1" {
		t.Errorf("ip = %q, want %q", ip, "127.0.0.1")
	}

	var detailMap map[string]interface{}
	if err := json.Unmarshal([]byte(detail), &detailMap); err != nil {
		t.Errorf("detail should be valid JSON: %v", err)
	}
}

func TestLogOperation_NilContext(t *testing.T) {
	db := setupAuditTestDB(t)
	defer db.Close()
	svc := newTestAuditService(db)

	// Should not panic
	svc.LogOperation(nil, 1, "admin", "TEST", "/test", nil, "127.0.0.1", "agent")
	svc.Close()

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM audit_logs WHERE action = 'TEST'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 entry, got %d", count)
	}
}

// --- TestLogSecurityEvent ---

func TestLogSecurityEvent(t *testing.T) {
	db := setupAuditTestDB(t)
	defer db.Close()
	svc := newTestAuditService(db)

	svc.LogSecurityEvent(context.Background(), "admin", "登录成功")
	svc.Close()

	// action column is the coarse verb ("认证"); summary is the human-readable text.
	// Operation logs do not record IP/user-agent (those are request-log concerns).
	var action, resource, ip, ua, detail string
	err := db.QueryRow("SELECT action, resource, ip, user_agent, detail FROM audit_logs WHERE type='operation'").
		Scan(&action, &resource, &ip, &ua, &detail)
	if err != nil {
		t.Fatalf("query audit_logs: %v", err)
	}
	if action != "认证" {
		t.Errorf("action = %q, want %q", action, "认证")
	}
	if resource != "/auth" {
		t.Errorf("resource = %q, want %q", resource, "/auth")
	}
	if ip != "" || ua != "" {
		t.Errorf("operation log ip/ua should be empty, got ip=%q ua=%q", ip, ua)
	}
	var d map[string]interface{}
	if err := json.Unmarshal([]byte(detail), &d); err != nil {
		t.Fatalf("detail not JSON: %v", err)
	}
	if d["summary"] != "登录成功" {
		t.Errorf("detail.summary = %v, want %q", d["summary"], "登录成功")
	}
}

// --- TestLogSystemEvent ---

func TestLogSystemEvent(t *testing.T) {
	db := setupAuditTestDB(t)
	defer db.Close()
	svc := newTestAuditService(db)

	svc.LogSystemEvent(context.Background(), "磁盘使用率告警：/ 95%")
	svc.Close()

	var action, detail string
	err := db.QueryRow("SELECT action, detail FROM audit_logs WHERE type='operation'").
		Scan(&action, &detail)
	if err != nil {
		t.Fatalf("query audit_logs: %v", err)
	}
	if action != "其他" {
		t.Errorf("action = %q, want %q", action, "其他")
	}
	var d map[string]interface{}
	json.Unmarshal([]byte(detail), &d)
	if d["summary"] != "磁盘使用率告警：/ 95%" {
		t.Errorf("detail.summary = %v, want %q", d["summary"], "磁盘使用率告警：/ 95%")
	}
}

// --- TestFlush ---

func TestFlush(t *testing.T) {
	db := setupAuditTestDB(t)
	defer db.Close()
	svc := newTestAuditService(db)

	entries := []auditEntry{
		{userID: 1, username: "admin", action: "ACTION1", resource: "/r1", detail: "{}", ip: "127.0.0.1", userAgent: "agent", logType: "operation", createdAt: time.Now()},
		{userID: 2, username: "user", action: "ACTION2", resource: "/r2", detail: "{}", ip: "10.0.0.1", userAgent: "agent", logType: "operation", createdAt: time.Now()},
	}

	svc.writer.flush(entries)

	// Verify entries were written
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM audit_logs").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("expected 2 entries, got %d", count)
	}

	// Verify type was persisted
	var logType string
	err = db.QueryRow("SELECT type FROM audit_logs WHERE id = 1").Scan(&logType)
	if err != nil {
		t.Fatal(err)
	}
	if logType != "operation" {
		t.Errorf("type = %q, want %q", logType, "operation")
	}
}
