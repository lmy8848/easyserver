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
	SetMustChangePass(ctx context.Context, id int64, mustChange bool) error
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
