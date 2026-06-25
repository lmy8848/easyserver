package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"github.com/skip2/go-qrcode"
	"golang.org/x/crypto/bcrypt"
)

const (
	// TOTP settings
	totpIssuer       = "EasyServer"
	totpPeriod       = 30 // seconds
	totpDigits       = 6
	totpAlgorithm    = "SHA1"
	totpSecretLength = 20 // bytes

	// Backup code settings
	backupCodeLength = 8
	backupCodeCount  = 10
)

// TOTPService handles TOTP operations
type TOTPService struct {
	db *sql.DB
}

// NewTOTPService creates a new TOTPService
func NewTOTPService(db *sql.DB) *TOTPService {
	return &TOTPService{db: db}
}

// TOTPSetupResult contains the TOTP setup information
type TOTPSetupResult struct {
	Secret      string `json:"secret"`
	OtpauthURL  string `json:"otpauth_url"`
	QRCodeBase64 string `json:"qr_code_base64"`
}

// GenerateTOTP generates a new TOTP secret and QR code for setup
func (s *TOTPService) GenerateTOTP(userID int64, username string) (*TOTPSetupResult, error) {
	// Generate TOTP key
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      totpIssuer,
		AccountName: username,
		Period:      totpPeriod,
		Digits:      totpDigits,
		Algorithm:   otp.AlgorithmSHA1,
		SecretSize:  totpSecretLength,
	})
	if err != nil {
		return nil, fmt.Errorf("generate TOTP key: %w", err)
	}

	// Generate QR code
	qrCode, err := qrcode.Encode(key.URL(), qrcode.Medium, 256)
	if err != nil {
		return nil, fmt.Errorf("generate QR code: %w", err)
	}

	// Encode QR code to base64
	qrCodeBase64 := fmt.Sprintf("data:image/png;base64,%s", base64Encode(qrCode))

	return &TOTPSetupResult{
		Secret:       key.Secret(),
		OtpauthURL:   key.URL(),
		QRCodeBase64: qrCodeBase64,
	}, nil
}

// VerifyTOTP verifies a TOTP code against a secret
func (s *TOTPService) VerifyTOTP(secret, code string) bool {
	return totp.Validate(code, secret)
}

// EnableTOTP enables 2FA for a user after verifying the code
func (s *TOTPService) EnableTOTP(ctx context.Context, userID int64, secret, code string) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	// Verify the code first
	if !s.VerifyTOTP(secret, code) {
		return nil, fmt.Errorf("invalid TOTP code")
	}

	// Generate backup codes
	backupCodes, err := s.GenerateBackupCodes()
	if err != nil {
		return nil, fmt.Errorf("generate backup codes: %w", err)
	}

	// Hash backup codes for storage
	hashedCodes := make([]string, len(backupCodes))
	for i, code := range backupCodes {
		hash, err := hashPassword(code)
		if err != nil {
			return nil, fmt.Errorf("hash backup code: %w", err)
		}
		hashedCodes[i] = hash
	}

	// Store hashed codes as JSON
	hashedCodesJSON, err := json.Marshal(hashedCodes)
	if err != nil {
		return nil, fmt.Errorf("marshal backup codes: %w", err)
	}

	// Update user in database
	_, err = s.db.ExecContext(ctx, `
		UPDATE users SET totp_secret = ?, totp_enabled = 1, totp_backup_codes = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, secret, string(hashedCodesJSON), userID)
	if err != nil {
		return nil, fmt.Errorf("update user TOTP: %w", err)
	}

	return backupCodes, nil
}

// DisableTOTP disables 2FA for a user after verifying the password
func (s *TOTPService) DisableTOTP(ctx context.Context, userID int64, password string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	// Get user's current password hash
	var passwordHash string
	err := s.db.QueryRowContext(ctx, "SELECT password_hash FROM users WHERE id = ?", userID).Scan(&passwordHash)
	if err != nil {
		return fmt.Errorf("get user password: %w", err)
	}

	// Verify password
	if !verifyPassword(password, passwordHash) {
		return fmt.Errorf("invalid password")
	}

	// Disable TOTP
	_, err = s.db.ExecContext(ctx, `
		UPDATE users SET totp_secret = '', totp_enabled = 0, totp_backup_codes = '[]', updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, userID)
	if err != nil {
		return fmt.Errorf("disable TOTP: %w", err)
	}

	return nil
}

// GenerateBackupCodes generates random backup codes
func (s *TOTPService) GenerateBackupCodes() ([]string, error) {
	codes := make([]string, backupCodeCount)
	for i := 0; i < backupCodeCount; i++ {
		code, err := generateRandomCode(backupCodeLength)
		if err != nil {
			return nil, fmt.Errorf("generate random code: %w", err)
		}
		codes[i] = code
	}
	return codes, nil
}

// VerifyBackupCode verifies and consumes a backup code
func (s *TOTPService) VerifyBackupCode(ctx context.Context, userID int64, code string) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	// Get user's backup codes
	var backupCodesJSON string
	err := s.db.QueryRowContext(ctx, "SELECT totp_backup_codes FROM users WHERE id = ? AND totp_enabled = 1", userID).Scan(&backupCodesJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, fmt.Errorf("user not found or 2FA not enabled")
		}
		return false, fmt.Errorf("get backup codes: %w", err)
	}

	// Parse backup codes
	var hashedCodes []string
	if err := json.Unmarshal([]byte(backupCodesJSON), &hashedCodes); err != nil {
		return false, fmt.Errorf("parse backup codes: %w", err)
	}

	// Try to verify each backup code
	for i, hashedCode := range hashedCodes {
		if verifyPassword(code, hashedCode) {
			// Remove used backup code
			hashedCodes = append(hashedCodes[:i], hashedCodes[i+1:]...)
			newJSON, err := json.Marshal(hashedCodes)
			if err != nil {
				return false, fmt.Errorf("marshal updated backup codes: %w", err)
			}

			// Update database
			_, err = s.db.ExecContext(ctx, `
				UPDATE users SET totp_backup_codes = ?, updated_at = CURRENT_TIMESTAMP
				WHERE id = ?
			`, string(newJSON), userID)
			if err != nil {
				return false, fmt.Errorf("update backup codes: %w", err)
			}

			return true, nil
		}
	}

	return false, nil
}

// IsTOTPEnabled checks if 2FA is enabled for a user
func (s *TOTPService) IsTOTPEnabled(ctx context.Context, userID int64) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var enabled bool
	err := s.db.QueryRowContext(ctx, "SELECT totp_enabled FROM users WHERE id = ?", userID).Scan(&enabled)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("check TOTP status: %w", err)
	}
	return enabled, nil
}

// GetTOTPSecret gets the TOTP secret for a user
func (s *TOTPService) GetTOTPSecret(ctx context.Context, userID int64) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var secret string
	err := s.db.QueryRowContext(ctx, "SELECT totp_secret FROM users WHERE id = ? AND totp_enabled = 1", userID).Scan(&secret)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("user not found or 2FA not enabled")
		}
		return "", fmt.Errorf("get TOTP secret: %w", err)
	}
	return secret, nil
}

// GetPendingSecret gets the pending TOTP secret for a user (during setup)
func (s *TOTPService) GetPendingSecret(ctx context.Context, userID int64) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var secret string
	err := s.db.QueryRowContext(ctx, "SELECT totp_secret FROM users WHERE id = ? AND totp_enabled = 0 AND totp_secret != ''", userID).Scan(&secret)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("no pending TOTP secret found")
		}
		return "", fmt.Errorf("get pending secret: %w", err)
	}
	return secret, nil
}

// StorePendingSecret stores a TOTP secret temporarily during setup
func (s *TOTPService) StorePendingSecret(ctx context.Context, userID int64, secret string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE users SET totp_secret = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, secret, userID)
	if err != nil {
		return fmt.Errorf("store pending secret: %w", err)
	}
	return nil
}

// Helper functions

func base64Encode(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func generateRandomCode(length int) (string, error) {
	const digits = "0123456789"
	code := make([]byte, length)
	for i := 0; i < length; i++ {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		if err != nil {
			return "", err
		}
		code[i] = digits[num.Int64()]
	}
	return string(code), nil
}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func verifyPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
