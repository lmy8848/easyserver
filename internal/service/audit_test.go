package service

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

func newTestAuditService(db *sql.DB) *AuditService {
	key := []byte("test-signing-key-32-chars-long!!")
	return &AuditService{
		db:            db,
		signingKey:    key,
		retentionDays: 90,
		writer: &AuditWriter{
			db:         db,
			ch:         make(chan auditEntry, 100),
			done:       make(chan struct{}),
			finished:   make(chan struct{}),
			signingKey: key,
		},
	}
}

// --- TestSignEntry ---

func TestSignEntry_Deterministic(t *testing.T) {
	svc := newTestAuditService(nil)
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
	svc := newTestAuditService(nil)
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

	// LogOperation sends to writer channel; we need to drain it manually
	svc.LogOperation(context.Background(), 1, "admin", "TEST_ACTION", "/test", "test detail", "127.0.0.1", "test-agent")

	// Drain the channel and flush manually
	batch := make([]auditEntry, 0)
	for {
		select {
		case entry := <-svc.writer.ch:
			batch = append(batch, entry)
		default:
			goto done
		}
	}
done:
	if len(batch) != 1 {
		t.Fatalf("expected 1 entry in channel, got %d", len(batch))
	}

	entry := batch[0]
	if entry.userID != 1 {
		t.Errorf("userID = %d, want 1", entry.userID)
	}
	if entry.username != "admin" {
		t.Errorf("username = %q, want %q", entry.username, "admin")
	}
	if entry.action != "TEST_ACTION" {
		t.Errorf("action = %q, want %q", entry.action, "TEST_ACTION")
	}
	if entry.ip != "127.0.0.1" {
		t.Errorf("ip = %q, want %q", entry.ip, "127.0.0.1")
	}

	// Verify detail is valid JSON
	var detailMap map[string]interface{}
	if err := json.Unmarshal([]byte(entry.detail), &detailMap); err != nil {
		t.Errorf("detail should be valid JSON: %v", err)
	}
}

func TestLogOperation_NilContext(t *testing.T) {
	db := setupAuditTestDB(t)
	defer db.Close()
	svc := newTestAuditService(db)

	// Should not panic
	svc.LogOperation(nil, 1, "admin", "TEST", "/test", "detail", "127.0.0.1", "agent")

	// Drain channel
	select {
	case <-svc.writer.ch:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected entry in channel")
	}
}

// --- TestLogSecurityEvent ---

func TestLogSecurityEvent(t *testing.T) {
	db := setupAuditTestDB(t)
	defer db.Close()
	svc := newTestAuditService(db)

	svc.LogSecurityEvent(context.Background(), "admin", "LOGIN_FAILED", "wrong password", "192.168.1.1", "Mozilla/5.0")

	select {
	case entry := <-svc.writer.ch:
		if entry.action != "SECURITY_LOGIN_FAILED" {
			t.Errorf("action = %q, want %q", entry.action, "SECURITY_LOGIN_FAILED")
		}
		if entry.resource != "/auth" {
			t.Errorf("resource = %q, want %q", entry.resource, "/auth")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected entry in channel")
	}
}

// --- TestLogTerminalCommand ---

func TestLogTerminalCommand(t *testing.T) {
	db := setupAuditTestDB(t)
	defer db.Close()
	svc := newTestAuditService(db)

	svc.LogTerminalCommand(context.Background(), 1, "admin", "sess-123", "ls -la", "127.0.0.1")

	select {
	case entry := <-svc.writer.ch:
		if entry.action != "TERMINAL" {
			t.Errorf("action = %q, want %q", entry.action, "TERMINAL")
		}
		if entry.resource != "/terminal/sess-123" {
			t.Errorf("resource = %q, want %q", entry.resource, "/terminal/sess-123")
		}
		// Verify detail contains command
		var detailMap map[string]interface{}
		json.Unmarshal([]byte(entry.detail), &detailMap)
		if detailMap["command"] != "ls -la" {
			t.Errorf("command = %v, want %q", detailMap["command"], "ls -la")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected entry in channel")
	}
}

// --- TestFlush ---

func TestFlush(t *testing.T) {
	db := setupAuditTestDB(t)
	defer db.Close()
	svc := newTestAuditService(db)

	entries := []auditEntry{
		{userID: 1, username: "admin", action: "ACTION1", resource: "/r1", detail: "{}", ip: "127.0.0.1", userAgent: "agent", createdAt: time.Now()},
		{userID: 2, username: "user", action: "ACTION2", resource: "/r2", detail: "{}", ip: "10.0.0.1", userAgent: "agent", createdAt: time.Now()},
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
}

// --- TestVerifySignature ---

func TestVerifySignature(t *testing.T) {
	db := setupAuditTestDB(t)
	defer db.Close()
	svc := newTestAuditService(db)

	// Insert an entry via flush
	entry := auditEntry{userID: 1, username: "admin", action: "TEST", resource: "/test", detail: "{}", ip: "127.0.0.1", userAgent: "agent", createdAt: time.Now()}
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
