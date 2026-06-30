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
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			signature TEXT DEFAULT ''
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

// --- TestSignEntry ---

func TestSignEntry_Deterministic(t *testing.T) {
	db := setupAuditTestDB(t)
	defer db.Close()
	svc := newTestAuditService(db)
	entry := auditEntry{
		userID:    1,
		username:  "admin",
		action:    "LOGIN",
		resource:  "/auth",
		detail:    `{"status":200}`,
		ip:        "127.0.0.1",
		userAgent: "test-agent",
		createdAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	sig1 := svc.writer.signEntry(entry)
	sig2 := svc.writer.signEntry(entry)

	if sig1 != sig2 {
		t.Errorf("signEntry should be deterministic: %q != %q", sig1, sig2)
	}
	if len(sig1) != 64 { // SHA-256 hex = 64 chars
		t.Errorf("signature length = %d, want 64", len(sig1))
	}
}

func TestSignEntry_DifferentEntriesProduceDifferentSignatures(t *testing.T) {
	db := setupAuditTestDB(t)
	defer db.Close()
	svc := newTestAuditService(db)
	now := time.Now()

	entry1 := auditEntry{userID: 1, username: "admin", action: "LOGIN", resource: "/auth", detail: "{}", ip: "127.0.0.1", userAgent: "agent", createdAt: now}
	entry2 := auditEntry{userID: 2, username: "admin", action: "LOGIN", resource: "/auth", detail: "{}", ip: "127.0.0.1", userAgent: "agent", createdAt: now}

	sig1 := svc.writer.signEntry(entry1)
	sig2 := svc.writer.signEntry(entry2)

	if sig1 == sig2 {
		t.Error("different entries should produce different signatures")
	}
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

	svc.LogSecurityEvent(context.Background(), "admin", "LOGIN_FAILED", "wrong password", "192.168.1.1", "Mozilla/5.0")
	svc.Close()

	var action, resource string
	err := db.QueryRow("SELECT action, resource FROM audit_logs WHERE action = 'SECURITY_LOGIN_FAILED'").
		Scan(&action, &resource)
	if err != nil {
		t.Fatalf("query audit_logs: %v", err)
	}
	if action != "SECURITY_LOGIN_FAILED" {
		t.Errorf("action = %q, want %q", action, "SECURITY_LOGIN_FAILED")
	}
	if resource != "/auth" {
		t.Errorf("resource = %q, want %q", resource, "/auth")
	}
}

// --- TestLogTerminalCommand ---

func TestLogTerminalCommand(t *testing.T) {
	db := setupAuditTestDB(t)
	defer db.Close()
	svc := newTestAuditService(db)

	svc.LogTerminalCommand(context.Background(), 1, "admin", "sess-123", "ls -la", "127.0.0.1")
	svc.Close()

	var action, resource, detail string
	err := db.QueryRow("SELECT action, resource, detail FROM audit_logs WHERE action = 'TERMINAL'").
		Scan(&action, &resource, &detail)
	if err != nil {
		t.Fatalf("query audit_logs: %v", err)
	}
	if action != "TERMINAL" {
		t.Errorf("action = %q, want %q", action, "TERMINAL")
	}
	if resource != "/terminal/sess-123" {
		t.Errorf("resource = %q, want %q", resource, "/terminal/sess-123")
	}
	var detailMap map[string]interface{}
	json.Unmarshal([]byte(detail), &detailMap)
	if detailMap["command"] != "ls -la" {
		t.Errorf("command = %v, want %q", detailMap["command"], "ls -la")
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

	// Verify signatures were generated
	var sig string
	err = db.QueryRow("SELECT signature FROM audit_logs WHERE id = 1").Scan(&sig)
	if err != nil {
		t.Fatal(err)
	}
	if len(sig) != 64 {
		t.Errorf("signature length = %d, want 64", len(sig))
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

// --- TestVerifySignature ---

func TestVerifySignature(t *testing.T) {
	db := setupAuditTestDB(t)
	defer db.Close()
	svc := newTestAuditService(db)

	// Insert an entry via flush
	entry := auditEntry{userID: 1, username: "admin", action: "TEST", resource: "/test", detail: "{}", ip: "127.0.0.1", userAgent: "agent", logType: "operation", createdAt: time.Now()}
	svc.writer.flush([]auditEntry{entry})

	// Verify the signature
	valid, err := svc.VerifySignature(context.Background(), 1)
	if err != nil {
		t.Fatalf("VerifySignature failed: %v", err)
	}
	if !valid {
		t.Error("signature should be valid")
	}

	// Tamper with the entry
	_, err = db.Exec("UPDATE audit_logs SET username = 'tampered' WHERE id = 1")
	if err != nil {
		t.Fatal(err)
	}

	valid, err = svc.VerifySignature(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if valid {
		t.Error("signature should be invalid after tampering")
	}
}
