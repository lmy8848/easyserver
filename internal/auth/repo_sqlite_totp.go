package auth

import (
	"context"
	"database/sql"
	"fmt"
)

// totpRepository implements TOTPRepository for SQLite.
type totpRepository struct {
	db *sql.DB
}

// NewTOTPRepository creates a new TOTPRepo.
func NewTOTPRepository(db *sql.DB) TOTPRepo {
	return &totpRepository{db: db}
}

// EnableTOTP enables 2FA for a user by storing the secret and hashed backup codes.
func (r *totpRepository) EnableTOTP(ctx context.Context, userID int64, secret string, hashedCodesJSON string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE users SET totp_secret = ?, totp_enabled = 1, totp_backup_codes = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, secret, hashedCodesJSON, userID)
	if err != nil {
		return fmt.Errorf("update user TOTP: %w", err)
	}
	return nil
}

// DisableTOTP disables 2FA for a user by clearing TOTP fields.
func (r *totpRepository) DisableTOTP(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE users SET totp_secret = '', totp_enabled = 0, totp_backup_codes = '[]', updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, userID)
	if err != nil {
		return fmt.Errorf("disable TOTP: %w", err)
	}
	return nil
}

// GetPasswordHash returns the password hash for a user.
func (r *totpRepository) GetPasswordHash(ctx context.Context, userID int64) (string, error) {
	var hash string
	err := r.db.QueryRowContext(ctx, "SELECT password_hash FROM users WHERE id = ?", userID).Scan(&hash)
	if err != nil {
		return "", fmt.Errorf("get user password: %w", err)
	}
	return hash, nil
}

// GetBackupCodes returns the backup codes JSON for a user with TOTP enabled.
func (r *totpRepository) GetBackupCodes(ctx context.Context, userID int64) (string, error) {
	var codesJSON string
	err := r.db.QueryRowContext(ctx, "SELECT totp_backup_codes FROM users WHERE id = ? AND totp_enabled = 1", userID).Scan(&codesJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("user not found or 2FA not enabled")
		}
		return "", fmt.Errorf("get backup codes: %w", err)
	}
	return codesJSON, nil
}

// UpdateBackupCodes updates the backup codes JSON for a user.
func (r *totpRepository) UpdateBackupCodes(ctx context.Context, userID int64, codesJSON string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE users SET totp_backup_codes = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, codesJSON, userID)
	if err != nil {
		return fmt.Errorf("update backup codes: %w", err)
	}
	return nil
}

// IsTOTPEnabled checks if 2FA is enabled for a user.
func (r *totpRepository) IsTOTPEnabled(ctx context.Context, userID int64) (bool, error) {
	var enabled bool
	err := r.db.QueryRowContext(ctx, "SELECT totp_enabled FROM users WHERE id = ?", userID).Scan(&enabled)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("check TOTP status: %w", err)
	}
	return enabled, nil
}

// GetTOTPSecret returns the TOTP secret for a user with TOTP enabled.
func (r *totpRepository) GetTOTPSecret(ctx context.Context, userID int64) (string, error) {
	var secret string
	err := r.db.QueryRowContext(ctx, "SELECT totp_secret FROM users WHERE id = ? AND totp_enabled = 1", userID).Scan(&secret)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("user not found or 2FA not enabled")
		}
		return "", fmt.Errorf("get TOTP secret: %w", err)
	}
	return secret, nil
}

// GetPendingSecret returns a pending TOTP secret for a user during setup.
func (r *totpRepository) GetPendingSecret(ctx context.Context, userID int64) (string, error) {
	var secret string
	err := r.db.QueryRowContext(ctx, "SELECT totp_secret FROM users WHERE id = ? AND totp_enabled = 0 AND totp_secret != ''", userID).Scan(&secret)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("no pending TOTP secret found")
		}
		return "", fmt.Errorf("get pending secret: %w", err)
	}
	return secret, nil
}

// StorePendingSecret stores a TOTP secret temporarily during setup.
func (r *totpRepository) StorePendingSecret(ctx context.Context, userID int64, secret string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE users SET totp_secret = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, secret, userID)
	if err != nil {
		return fmt.Errorf("store pending secret: %w", err)
	}
	return nil
}
