package model

import (
	"database/sql"
	"time"
)

type Role string

const (
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleViewer   Role = "viewer"
)

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

// UserActivity represents a user activity log entry
type UserActivity struct {
	ID        int64     `json:"id" db:"id"`
	UserID    int64     `json:"user_id" db:"user_id"`
	Username  string    `json:"username" db:"username"`
	Action    string    `json:"action" db:"action"`
	IP        string    `json:"ip" db:"ip"`
	UserAgent string    `json:"user_agent" db:"user_agent"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// Session represents an active user session
type Session struct {
	UserID    int64     `json:"user_id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"user_agent"`
	LoginAt   time.Time `json:"login_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Token     string    `json:"token,omitempty"`
}
