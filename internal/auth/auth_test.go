package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

// --- TestValidatePassword ---

func TestValidatePassword(t *testing.T) {
	s := &AuthService{}

	tests := []struct {
		name     string
		password string
		wantErr  bool
		errMsg   string
	}{
		{"valid password", "Abcdef12", false, ""},
		{"valid with special chars", "Abc@1234!", false, ""},
		{"too short", "Abc12", true, "password must be at least 8 characters"},
		{"exactly 7 chars", "Abc1234", true, "password must be at least 8 characters"},
		{"exactly 8 chars valid", "Abcdefg1", false, ""},
		{"too long (>128)", string(make([]byte, 129)), true, "password must be less than 128 characters"},
		{"no uppercase", "abcdefg1", true, "password must contain upper, lower case and digit"},
		{"no lowercase", "ABCDEFG1", true, "password must contain upper, lower case and digit"},
		{"no digit", "Abcdefgh", true, "password must contain upper, lower case and digit"},
		{"only digits", "12345678", true, "password must contain upper, lower case and digit"},
		{"only lowercase", "abcdefgh", true, "password must contain upper, lower case and digit"},
		{"only uppercase", "ABCDEFGH", true, "password must contain upper, lower case and digit"},
		{"empty string", "", true, "password must be at least 8 characters"},
		{"128 chars valid", buildPassword(128, true, true, true), false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := s.ValidatePassword(tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePassword(%q) error = %v, wantErr %v", tt.password, err, tt.wantErr)
			}
			if tt.wantErr && err != nil && err.Error() != tt.errMsg {
				t.Errorf("ValidatePassword(%q) error message = %q, want %q", tt.password, err.Error(), tt.errMsg)
			}
		})
	}
}

func buildPassword(length int, hasUpper, hasLower, hasDigit bool) string {
	b := make([]byte, length)
	for i := range b {
		if hasUpper && i%3 == 0 {
			b[i] = 'A'
		} else if hasLower && i%3 == 1 {
			b[i] = 'a'
		} else if hasDigit && i%3 == 2 {
			b[i] = '1'
		} else {
			b[i] = 'x'
		}
	}
	// Ensure all required types are present
	if hasUpper {
		b[0] = 'A'
	}
	if hasLower {
		b[1] = 'a'
	}
	if hasDigit {
		b[2] = '1'
	}
	return string(b)
}

// --- TestHashToken ---

func TestHashToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{"simple token", "abc123"},
		{"jwt-like token", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"},
		{"empty string", ""},
		{"special characters", "!@#$%^&*()"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hashToken(tt.token)

			// Verify it's a valid hex string
			if len(result) != 64 { // SHA-256 produces 32 bytes = 64 hex chars
				t.Errorf("hashToken(%q) length = %d, want 64", tt.token, len(result))
			}

			// Verify it matches manual SHA-256
			h := sha256.Sum256([]byte(tt.token))
			expected := hex.EncodeToString(h[:])
			if result != expected {
				t.Errorf("hashToken(%q) = %q, want %q", tt.token, result, expected)
			}
		})
	}
}

func TestHashToken_Deterministic(t *testing.T) {
	token := "test-token-123"
	h1 := hashToken(token)
	h2 := hashToken(token)
	if h1 != h2 {
		t.Errorf("hashToken should be deterministic: %q != %q", h1, h2)
	}
}

func TestHashToken_DifferentInputsProduceDifferentHashes(t *testing.T) {
	h1 := hashToken("token-a")
	h2 := hashToken("token-b")
	if h1 == h2 {
		t.Error("different tokens should produce different hashes")
	}
}

// --- helpers for DB-dependent tests ---

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	// Create required tables
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'admin',
			must_change_pass INTEGER DEFAULT 0,
			last_login DATETIME,
			last_login_ip TEXT DEFAULT '',
			login_attempts INTEGER DEFAULT 0,
			locked_until DATETIME,
			expires_at DATETIME,
			ip_whitelist TEXT DEFAULT '',
			totp_secret TEXT DEFAULT '',
			totp_enabled INTEGER DEFAULT 0,
			totp_backup_codes TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS token_blacklist (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			token TEXT NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS user_activities (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			username TEXT NOT NULL,
			action TEXT NOT NULL,
			ip TEXT DEFAULT '',
			user_agent TEXT DEFAULT '',
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

func createTestUser(t *testing.T, db *sql.DB, username, password string, locked bool) int64 {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	var lockedUntil interface{}
	if locked {
		lockedUntil = time.Now().Add(1 * time.Hour).Format("2006-01-02 15:04:05")
	}
	result, err := db.Exec(
		"INSERT INTO users (username, password_hash, role, locked_until) VALUES (?, ?, 'admin', ?)",
		username, string(hash), lockedUntil,
	)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := result.LastInsertId()
	return id
}

func newTestAuthService(db *sql.DB) *AuthService {
	return &AuthService{
		userRepo:        NewSQLiteUserRepository(db),
		tokenRepo:       NewSQLiteTokenRepository(db),
		maxAttempts:     5,
		lockoutDuration: 5 * time.Minute,
	}
}

// --- TestLogin ---

func TestLogin_Success(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	svc := newTestAuthService(db)
	createTestUser(t, db, "admin", "Admin123", false)

	user, err := svc.Login(context.Background(), "admin", "Admin123")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if user.Username != "admin" {
		t.Errorf("Username = %q, want %q", user.Username, "admin")
	}
	if user.Role != "admin" {
		t.Errorf("Role = %q, want %q", user.Role, "admin")
	}
}

func TestLogin_InvalidPassword(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	svc := newTestAuthService(db)
	createTestUser(t, db, "admin", "Admin123", false)

	_, err := svc.Login(context.Background(), "admin", "WrongPass1")
	if err == nil {
		t.Fatal("expected error for wrong password")
	}
	if err.Error() != "用户名或密码错误" {
		t.Errorf("error = %q, want %q", err.Error(), "用户名或密码错误")
	}
}

func TestLogin_UserNotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	svc := newTestAuthService(db)

	_, err := svc.Login(context.Background(), "nonexistent", "Admin123")
	if err == nil {
		t.Fatal("expected error for non-existent user")
	}
	if err.Error() != "用户名或密码错误" {
		t.Errorf("error = %q, want %q", err.Error(), "用户名或密码错误")
	}
}

func TestLogin_AccountLocked(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	svc := newTestAuthService(db)
	createTestUser(t, db, "admin", "Admin123", true) // locked = true

	_, err := svc.Login(context.Background(), "admin", "Admin123")
	if err == nil {
		t.Fatal("expected error for locked account")
	}
	if err.Error() != "账号已被锁定" {
		t.Errorf("error = %q, want %q", err.Error(), "账号已被锁定")
	}
}

func TestLogin_NilContext(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	svc := newTestAuthService(db)
	createTestUser(t, db, "admin", "Admin123", false)

	// Should not panic with nil context
	user, err := svc.Login(nil, "admin", "Admin123")
	if err != nil {
		t.Fatalf("Login with nil context failed: %v", err)
	}
	if user.Username != "admin" {
		t.Errorf("Username = %q, want %q", user.Username, "admin")
	}
}

// --- TestChangePassword ---

func TestChangePassword_Success(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	svc := newTestAuthService(db)
	userID := createTestUser(t, db, "admin", "Admin123", false)

	err := svc.ChangePassword(context.Background(), userID, "Admin123", "NewPass456")
	if err != nil {
		t.Fatalf("ChangePassword failed: %v", err)
	}

	// Verify old password no longer works
	_, err = svc.Login(context.Background(), "admin", "Admin123")
	if err == nil {
		t.Error("old password should no longer work")
	}

	// Verify new password works
	user, err := svc.Login(context.Background(), "admin", "NewPass456")
	if err != nil {
		t.Fatalf("Login with new password failed: %v", err)
	}
	if user.Username != "admin" {
		t.Errorf("Username = %q, want %q", user.Username, "admin")
	}
}

func TestChangePassword_WrongOldPassword(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	svc := newTestAuthService(db)
	userID := createTestUser(t, db, "admin", "Admin123", false)

	err := svc.ChangePassword(context.Background(), userID, "WrongOld1", "NewPass456")
	if err == nil {
		t.Fatal("expected error for wrong old password")
	}
	if err.Error() != "invalid old password" {
		t.Errorf("error = %q, want %q", err.Error(), "invalid old password")
	}
}

func TestChangePassword_AccountLocked(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	svc := newTestAuthService(db)
	userID := createTestUser(t, db, "admin", "Admin123", true) // locked

	err := svc.ChangePassword(context.Background(), userID, "Admin123", "NewPass456")
	if err == nil {
		t.Fatal("expected error for locked account")
	}
	if err.Error() != "account is locked" {
		t.Errorf("error = %q, want %q", err.Error(), "account is locked")
	}
}

func TestChangePassword_WeakNewPassword(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	svc := newTestAuthService(db)
	userID := createTestUser(t, db, "admin", "Admin123", false)

	err := svc.ChangePassword(context.Background(), userID, "Admin123", "weak")
	if err == nil {
		t.Fatal("expected error for weak new password")
	}
}

func TestChangePassword_UserNotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	svc := newTestAuthService(db)

	err := svc.ChangePassword(context.Background(), 99999, "Admin123", "NewPass456")
	if err == nil {
		t.Fatal("expected error for non-existent user")
	}
}

// --- TestAddTokenToBlacklist / TestIsTokenBlacklisted ---

func TestAddTokenToBlacklist(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	svc := newTestAuthService(db)

	token := "test-jwt-token-abc123"
	expiresAt := time.Now().Add(1 * time.Hour)

	err := svc.AddTokenToBlacklist(context.Background(), 1, token, expiresAt)
	if err != nil {
		t.Fatalf("AddTokenToBlacklist failed: %v", err)
	}

	// Verify the token hash is stored, not the raw token
	tokenHash := hashToken(token)
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM token_blacklist WHERE token = ?", tokenHash).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 blacklisted entry, got %d", count)
	}
}

func TestIsTokenBlacklisted_True(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	svc := newTestAuthService(db)

	token := "test-jwt-token-abc123"
	expiresAt := time.Now().Add(1 * time.Hour)

	err := svc.AddTokenToBlacklist(context.Background(), 1, token, expiresAt)
	if err != nil {
		t.Fatal(err)
	}

	blacklisted, err := svc.IsTokenBlacklisted(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if !blacklisted {
		t.Error("expected token to be blacklisted")
	}
}

func TestIsTokenBlacklisted_False(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	svc := newTestAuthService(db)

	blacklisted, err := svc.IsTokenBlacklisted(context.Background(), "unknown-token")
	if err != nil {
		t.Fatal(err)
	}
	if blacklisted {
		t.Error("expected token to NOT be blacklisted")
	}
}

func TestIsTokenBlacklisted_Expired(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	svc := newTestAuthService(db)

	token := "expired-token"
	expiresAt := time.Now().Add(-1 * time.Hour) // already expired

	err := svc.AddTokenToBlacklist(context.Background(), 1, token, expiresAt)
	if err != nil {
		t.Fatal(err)
	}

	blacklisted, err := svc.IsTokenBlacklisted(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if blacklisted {
		t.Error("expired token should not be considered blacklisted")
	}
}

func TestIsTokenBlacklisted_CacheHit(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	svc := newTestAuthService(db)

	token := "cached-token"
	expiresAt := time.Now().Add(1 * time.Hour)

	// Add to blacklist (which also caches)
	err := svc.AddTokenToBlacklist(context.Background(), 1, token, expiresAt)
	if err != nil {
		t.Fatal(err)
	}

	// Remove from DB to prove cache is being used
	tokenHash := hashToken(token)
	_, _ = db.Exec("DELETE FROM token_blacklist WHERE token = ?", tokenHash)

	blacklisted, err := svc.IsTokenBlacklisted(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if !blacklisted {
		t.Error("expected token to be blacklisted from cache")
	}
}

// --- TestInvalidateAllUserTokens ---

func TestInvalidateAllUserTokens(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	svc := newTestAuthService(db)

	userID := createTestUser(t, db, "admin", "Admin123", false)

	err := svc.InvalidateAllUserTokens(context.Background(), userID)
	if err != nil {
		t.Fatalf("InvalidateAllUserTokens failed: %v", err)
	}

	// Verify a marker was created
	var count int
	err = db.QueryRow(
		"SELECT COUNT(*) FROM token_blacklist WHERE user_id = ? AND token = ?",
		userID, "user_"+int64ToString(userID)+"_all",
	).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 invalidation marker, got %d", count)
	}
}

func TestInvalidateAllUserTokens_CacheEffect(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	svc := newTestAuthService(db)

	userID := createTestUser(t, db, "admin", "Admin123", false)

	// Record time before invalidation
	beforeInvalidation := time.Now()
	time.Sleep(10 * time.Millisecond)

	err := svc.InvalidateAllUserTokens(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}

	// Token issued before invalidation should be detected as invalidated
	invalidated, err := svc.IsUserTokenInvalidated(context.Background(), userID, beforeInvalidation)
	if err != nil {
		t.Fatal(err)
	}
	if !invalidated {
		t.Error("token issued before invalidation should be detected as invalidated")
	}

	// Token issued after invalidation should NOT be invalidated
	invalidated, err = svc.IsUserTokenInvalidated(context.Background(), userID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if invalidated {
		t.Error("token issued after invalidation should NOT be invalidated")
	}
}

func TestInvalidateAllUserTokens_NilContext(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	svc := newTestAuthService(db)
	userID := createTestUser(t, db, "admin", "Admin123", false)

	// Should not panic
	err := svc.InvalidateAllUserTokens(nil, userID)
	if err != nil {
		t.Fatalf("InvalidateAllUserTokens with nil context failed: %v", err)
	}
}

func int64ToString(n int64) string {
	return fmt.Sprintf("%d", n)
}
