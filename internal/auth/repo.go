package auth

import (
	"context"
)

type UserRepo interface {
	GetByID(ctx context.Context, id int64) (*User, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
	Create(ctx context.Context, user *User) error
	List(ctx context.Context, offset, limit int) ([]User, int64, error)
	UpdatePassword(ctx context.Context, id int64, passwordHash string) error
	UpdateUsername(ctx context.Context, id int64, username string) error
	IncrementLoginAttempts(ctx context.Context, id int64, maxAttempts int, lockoutSeconds int) (bool, error)
	ResetLoginState(ctx context.Context, id int64, ip string) error
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
}

// QRLoginRepository defines data access for QR login sessions.
type QRLoginRepository interface {
	Create(ctx context.Context, s *QRLoginSession) (int64, error)
	GetByToken(ctx context.Context, qrToken string) (*QRLoginSession, error)
	// MarkConfirmed stores the issued web token + user payload and transitions to confirmed.
	MarkConfirmed(ctx context.Context, qrToken string, userID int64, webToken string, userJSON string) error
	// Consume atomically claims a confirmed session via a conditional UPDATE
	// (confirmed -> consumed). It returns the session only if THIS caller won the
	// race; ok is false when another poll already claimed it, it was cancelled,
	// or it is still pending.
	Consume(ctx context.Context, qrToken string) (*QRLoginSession, bool, error)
	Delete(ctx context.Context, qrToken string) error
	DeleteExpired(ctx context.Context) (int64, error)
}
