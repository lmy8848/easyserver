package auth

import (
	"context"
	"database/sql"
	"time"
)

type Role string

const RoleAdmin Role = "admin"

type User struct {
	ID              int64        `json:"id" db:"id"`
	Username        string       `json:"username" db:"username"`
	PasswordHash    string       `json:"-" db:"password_hash"`
	Role            Role         `json:"role" db:"role"`
	MustChangePass  bool         `json:"must_change_pass" db:"must_change_pass"`
	LastLogin       sql.NullTime `json:"last_login" db:"last_login"`
	LastLoginIP     string       `json:"last_login_ip" db:"last_login_ip"`
	LoginAttempts   int          `json:"-" db:"login_attempts"`
	LockedUntil     sql.NullTime `json:"-" db:"locked_until"`
	ExpiresAt       sql.NullTime `json:"expires_at" db:"expires_at"`
	IPWhitelist     string       `json:"ip_whitelist" db:"ip_whitelist"`
	TotpSecret      string       `json:"-" db:"totp_secret"`
	TotpEnabled     bool         `json:"totp_enabled" db:"totp_enabled"`
	TotpBackupCodes string       `json:"-" db:"totp_backup_codes"`
	CreatedAt       time.Time    `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at" db:"updated_at"`
}

type Session struct {
	UserID     int64     `json:"user_id"`
	Username   string    `json:"username"`
	Role       string    `json:"role"`
	IP         string    `json:"ip"`
	UserAgent  string    `json:"user_agent"`
	ClientType string    `json:"client_type"`
	DeviceID   string    `json:"device_id,omitempty"`
	DeviceInfo string    `json:"device_info,omitempty"`
	LoginAt    time.Time `json:"login_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Token      string    `json:"token,omitempty"`
}

// LoginEvent represents a login event, used by both the audit logger
// (LoginEventLogger) and the webhook notifier (LoginNotifier).
type LoginEvent struct {
	Action    string `json:"action"` // LOGIN_SUCCESS, LOGIN_FAILED, LOGIN_BLOCKED_IP, LOGIN_BLOCKED_EXPIRED
	Username  string `json:"username"`
	IP        string `json:"ip"`
	UserAgent string `json:"user_agent"`
	Time      string `json:"time"`
	Success   bool   `json:"success"`
	Reason    string `json:"reason,omitempty"`
}

// LoginEventLogger records login events for later analysis (e.g. brute-force
// detection). *audit.Service satisfies this interface implicitly.
type LoginEventLogger interface {
	LogLoginEvent(ctx context.Context, event LoginEvent)
}

// LoginNotifier sends login notifications (webhook).
type LoginNotifier interface {
	NotifyLogin(event LoginEvent)
}
