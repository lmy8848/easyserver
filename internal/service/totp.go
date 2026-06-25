package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"

	"easyserver/internal/repository"

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
	repo repository.TOTPRepository
}

// NewTOTPService creates a new TOTPService
func NewTOTPService(repo repository.TOTPRepository) *TOTPService {
	return &TOTPService{repo: repo}
}

// TOTPSetupResult contains the TOTP setup information
type TOTPSetupResult struct {
	Secret       string `json:"secret"`
	OtpauthURL   string `json:"otpauth_url"`
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
	if err := s.repo.EnableTOTP(ctx, userID, secret, string(hashedCodesJSON)); err != nil {
		return nil, err
	}

	return backupCodes, nil
}

// DisableTOTP disables 2FA for a user after verifying the password
func (s *TOTPService) DisableTOTP(ctx context.Context, userID int64, password string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	// Get user's current password hash
	passwordHash, err := s.repo.GetPasswordHash(ctx, userID)
	if err != nil {
		return err
	}

	// Verify password
	if !verifyPassword(password, passwordHash) {
		return fmt.Errorf("invalid password")
	}

	// Disable TOTP
	return s.repo.DisableTOTP(ctx, userID)
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
	backupCodesJSON, err := s.repo.GetBackupCodes(ctx, userID)
	if err != nil {
		return false, err
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
			if err := s.repo.UpdateBackupCodes(ctx, userID, string(newJSON)); err != nil {
				return false, err
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
	return s.repo.IsTOTPEnabled(ctx, userID)
}

// GetTOTPSecret gets the TOTP secret for a user
func (s *TOTPService) GetTOTPSecret(ctx context.Context, userID int64) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.GetTOTPSecret(ctx, userID)
}

// GetPendingSecret gets the pending TOTP secret for a user (during setup)
func (s *TOTPService) GetPendingSecret(ctx context.Context, userID int64) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.GetPendingSecret(ctx, userID)
}

// StorePendingSecret stores a TOTP secret temporarily during setup
func (s *TOTPService) StorePendingSecret(ctx context.Context, userID int64, secret string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.StorePendingSecret(ctx, userID, secret)
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
