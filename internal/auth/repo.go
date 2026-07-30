package auth

import (
	"context"
	"database/sql"
	"time"
)

type UserRepo interface {
	GetByID(ctx context.Context, id int64) (*User, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
	Create(ctx context.Context, user *User) error
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, offset, limit int) ([]User, int64, error)
	UpdateLoginAttempts(ctx context.Context, id int64, attempts int, lockedUntil *time.Time) error
	UpdatePassword(ctx context.Context, id int64, passwordHash string) error
	SetMustChangePass(ctx context.Context, id int64, mustChange bool) error
	IncrementLoginAttempts(ctx context.Context, id int64) error
	IncrementLoginAttemptsWithLock(ctx context.Context, id int64, maxAttempts int, lockoutSeconds int) error
	ResetLoginState(ctx context.Context, id int64, ip string) error
	UpdateLastLoginIP(ctx context.Context, id int64, ip string) error
	SetAccountExpiry(ctx context.Context, id int64, expiresAt *time.Time) error
	GetAccountExpiry(ctx context.Context, id int64) (sql.NullTime, error)
	SetIPWhitelist(ctx context.Context, id int64, whitelist string) error
	GetIPWhitelist(ctx context.Context, id int64) (string, error)
}

type TokenBlacklistRepo interface {
	Add(ctx context.Context, userID int64, token string, expiresAt time.Time) error
	IsBlacklisted(ctx context.Context, token string) (bool, error)
	AddUserInvalidation(ctx context.Context, userID int64) error
	IsUserInvalidated(ctx context.Context, userID int64, issuedAt time.Time) (bool, error)
	Clean(ctx context.Context) error
}

type ActivityRepo interface {
	Log(ctx context.Context, entry *UserActivity) error
	GetByUserID(ctx context.Context, userID int64, limit int) ([]UserActivity, error)
	GetAll(ctx context.Context, limit int) ([]UserActivity, error)
}

// TOTPer is the subset of TOTPRepository that AuthService needs.
type TOTPer interface {
	IsTOTPEnabled(ctx context.Context, userID int64) (bool, error)
	GetTOTPSecret(ctx context.Context, userID int64) (string, error)
}

// TOTPRepo defines the interface for TOTP data access.
type TOTPRepo interface {
	TOTPer
	EnableTOTP(ctx context.Context, userID int64, secret string, hashedCodesJSON string) error
	DisableTOTP(ctx context.Context, userID int64) error
	GetPasswordHash(ctx context.Context, userID int64) (string, error)
	GetBackupCodes(ctx context.Context, userID int64) (string, error)
	UpdateBackupCodes(ctx context.Context, userID int64, codesJSON string) error
	GetPendingSecret(ctx context.Context, userID int64) (string, error)
	StorePendingSecret(ctx context.Context, userID int64, secret string) error
}
