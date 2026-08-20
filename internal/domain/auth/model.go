package auth

import (
	"context"
	"database/sql"
	"time"
)

type Role string

const RoleAdmin Role = "admin"

// Status constants for a QR login session's lifecycle.
const (
	QRStatusPending   = "pending"   // QR displayed, waiting for mobile to scan+confirm
	QRStatusConfirmed = "confirmed" // Mobile confirmed; web_token issued, awaiting web pickup
	QRStatusConsumed  = "consumed"  // A web client claimed the token (one-time); ready for cleanup
	QRStatusCancelled = "cancelled" // User cancelled before confirmation
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
	TotpSecret      string       `json:"-" db:"totp_secret"`
	TotpEnabled     bool         `json:"totp_enabled" db:"totp_enabled"`
	TotpBackupCodes string       `json:"-" db:"totp_backup_codes"`
	CreatedAt       time.Time    `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at" db:"updated_at"`
}

type Session struct {
	UserID     int64     `json:"user_id"`
	IP         string    `json:"ip"`
	UserAgent  string    `json:"user_agent"`
	ClientType string    `json:"client_type"`
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

// QRLoginSession is a one-time scan-to-login session (in-memory).
type QRLoginSession struct {
	QRToken   string    `json:"qr_token"`
	Status    string    `json:"status"`
	WebToken  string    `json:"-"` // never serialize the issued web token
	User      *User     `json:"-"` // 确认时的用户快照，web 领取时随状态返回
	ExpiresAt time.Time `json:"expires_at"`
}

// CreateResult is returned when a web client requests a new QR session.
type CreateResult struct {
	QRToken   string    `json:"qr_token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// StatusResult is returned to the polling web client. On the first confirmed
// poll the web_token is set (and consumed to a cookie server-side) along with
// the confirming user's snapshot, so the client doesn't need a follow-up call.
type StatusResult struct {
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
	Token     string    `json:"token,omitempty"`
	User      *User     `json:"user,omitempty"`
}

// ConfirmRequest is the body the mobile app sends after scanning the QR.
type QRLoginConfirmRequest struct {
	QRToken string `json:"qr_token" binding:"required"`
}
