package auth

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"easyserver/internal/infra/config"

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

// --- helpers for DB-dependent tests ---

func setupTestDB(t *testing.T) *sql.DB {
	ctx := context.Background()
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
			totp_secret TEXT DEFAULT '',
			totp_enabled INTEGER DEFAULT 0,
			totp_backup_codes TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}
	for _, q := range queries {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func createTestUser(t *testing.T, db *sql.DB, username, password string, locked bool) int64 {
	ctx := context.Background()
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	var lockedUntil any
	if locked {
		lockedUntil = time.Now().Add(1 * time.Hour).Format("2006-01-02 15:04:05")
	}
	result, err := db.ExecContext(ctx,
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
	store := config.NewStore(&config.Config{
		Auth: config.AuthConfig{
			MaxLoginAttempts: 5,
			LockoutDuration:  config.Duration(5 * time.Minute),
		},
	})
	return &AuthService{
		userRepo: NewSQLiteUserRepository(db),
		store:    store,
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

func TestLogin_TODOContext(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	svc := newTestAuthService(db)
	createTestUser(t, db, "admin", "Admin123", false)

	// 显式传空 context（不传 nil，符合 SA1012）
	user, err := svc.Login(context.TODO(), "admin", "Admin123")
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

func TestChangePassword_SamePassword(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	svc := newTestAuthService(db)
	userID := createTestUser(t, db, "admin", "Admin123", false)

	err := svc.ChangePassword(context.Background(), userID, "Admin123", "Admin123")
	if err == nil {
		t.Fatal("expected error when new password equals old password")
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

// --- TestChangeUsername ---

func TestChangeUsername_Success(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	svc := newTestAuthService(db)
	userID := createTestUser(t, db, "olduser", "Admin123", false)

	err := svc.ChangeUsername(context.Background(), userID, "newuser", "Admin123")
	if err != nil {
		t.Fatalf("ChangeUsername failed: %v", err)
	}

	// Verify old username no longer logs in
	_, err = svc.Login(context.Background(), "olduser", "Admin123")
	if err == nil {
		t.Error("old username should not log in")
	}

	// Verify new username logs in
	user, err := svc.Login(context.Background(), "newuser", "Admin123")
	if err != nil {
		t.Fatalf("Login with new username failed: %v", err)
	}
	if user.Username != "newuser" {
		t.Errorf("Username = %q, want %q", user.Username, "newuser")
	}
}

func TestChangeUsername_WrongPassword(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	svc := newTestAuthService(db)
	userID := createTestUser(t, db, "myuser", "Admin123", false)

	err := svc.ChangeUsername(context.Background(), userID, "newuser", "WrongPass")
	if err == nil {
		t.Fatal("expected error for wrong password")
	}
}

func TestChangeUsername_Duplicate(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	svc := newTestAuthService(db)
	userID := createTestUser(t, db, "user1", "Admin123", false)
	createTestUser(t, db, "user2", "Admin123", false)

	err := svc.ChangeUsername(context.Background(), userID, "user2", "Admin123")
	if err == nil {
		t.Fatal("expected error for duplicate username")
	}
}

func TestChangeUsername_InvalidFormat(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	svc := newTestAuthService(db)
	userID := createTestUser(t, db, "validuser", "Admin123", false)

	// too short
	if err := svc.ChangeUsername(context.Background(), userID, "ab", "Admin123"); err == nil {
		t.Error("expected error for short username")
	}

	// illegal characters
	if err := svc.ChangeUsername(context.Background(), userID, "bad user name!", "Admin123"); err == nil {
		t.Error("expected error for username with invalid characters")
	}
}
