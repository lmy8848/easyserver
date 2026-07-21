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

type SessionRepo interface {
	Create(ctx context.Context, session *Session) error
	GetByToken(ctx context.Context, token string) (*Session, error)
	DeleteByToken(ctx context.Context, token string) error
	DeleteByUserID(ctx context.Context, userID int64) error
	DeleteExpired(ctx context.Context) error
	DeleteInactive(ctx context.Context, inactiveSince time.Time) error
	DeleteByUserIDExcept(ctx context.Context, userID int64, exceptToken string) error
	// DeleteByStoredToken deletes a session by the already-hashed token value
	// stored in the DB (no re-hashing). Used by the kick path, which receives
	// the hash back from GetSessions, and by same-device mobile refresh.
	DeleteByStoredToken(ctx context.Context, storedToken string) error
	// DeleteMobileByUserID deletes all mobile sessions for a user (used to
	// refresh/replace the bound mobile device session).
	DeleteMobileByUserID(ctx context.Context, userID int64) error
	// DeleteMobileByUserIDExcept deletes all mobile sessions for a user except
	// the one whose token hashes to exceptToken (plaintext). Used by the
	// same-device refresh path to remove the old session after creating the new.
	DeleteMobileByUserIDExcept(ctx context.Context, userID int64, exceptToken string) error
	IsValid(ctx context.Context, token string) (bool, error)
	GetActiveByUserID(ctx context.Context, userID int64) ([]Session, error)
	GetActive(ctx context.Context) ([]Session, error)
	UpdateActivity(ctx context.Context, token string) error
	Count(ctx context.Context) (int, error)
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
