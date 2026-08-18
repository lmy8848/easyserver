package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net"
	"slices"
	"strings"
	"sync"
	"time"

	"easyserver/internal/infra/config"
	"easyserver/internal/infra/errx"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
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

type AuthService struct {
	store       *config.Store
	userRepo    UserRepo
	loginLogger LoginEventLogger
	totpRepo    TOTPRepo
	notifier    LoginNotifier
}

// NewAuthService 构造认证服务。登录限制参数（最大尝试次数/锁定时长）运行时
// 实时读 store：settings 修改后立即生效，无需重启。
func NewAuthService(ctx context.Context, wg *sync.WaitGroup, store *config.Store, userRepo UserRepo, loginLogger LoginEventLogger, totpRepo TOTPRepo, notifier LoginNotifier) *AuthService {
	s := &AuthService{
		store:       store,
		userRepo:    userRepo,
		loginLogger: loginLogger,
		totpRepo:    totpRepo,
		notifier:    notifier,
	}
	return s
}

// NotifyLogin exposes the login notifier so handlers can emit login events
// outside the normal LoginWithInfo flow (e.g. a mobile-binding rejection that
// happens after credentials were verified but before a session is created).
func (s *AuthService) NotifyLogin(event LoginEvent) {
	if s.notifier != nil {
		s.notifier.NotifyLogin(event)
	}
}

func (s *AuthService) InitDefaultAdmin(ctx context.Context) error {
	_, total, err := s.userRepo.List(ctx, 0, 1)
	if err != nil {
		return err
	}
	if total == 0 {
		password := generateRandomPassword(16)
		hash, err := hashPassword(password)
		if err != nil {
			return err
		}
		if err := s.userRepo.Create(ctx, &User{
			Username:       "admin",
			PasswordHash:   hash,
			Role:           RoleAdmin,
			MustChangePass: true,
		}); err != nil {
			return err
		}
		fmt.Println("=================================================")
		fmt.Println("EasyServer 初次启动 - 管理员账号信息")
		fmt.Println("=================================================")
		fmt.Printf("用户名: admin\n")
		fmt.Printf("密码:   %s\n", password)
		fmt.Println("=================================================")
		fmt.Println("请登录后立即修改密码！")
		fmt.Println("=================================================")
		return nil
	}
	return nil
}

func generateRandomPassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	charsetLen := byte(len(charset))
	limit := 256 - (256 % int(charsetLen))
	password := make([]byte, length)
	for i := range password {
		for {
			b := make([]byte, 1)
			if _, err := rand.Read(b); err != nil {
				continue
			}
			if int(b[0]) >= limit {
				continue
			}
			password[i] = charset[int(b[0])%int(charsetLen)]
			break
		}
	}
	return string(password)
}

func (s *AuthService) Login(ctx context.Context, username, password string) (*User, error) {
	user, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		return nil, errors.New("用户名或密码错误")
	}

	if user.LockedUntil.Valid && user.LockedUntil.Time.After(time.Now()) {
		if err := s.userRepo.IncrementLoginAttempts(ctx, user.ID); err != nil {
			log.Printf("auth: failed to increment login attempts: %v", err)
		}
		return nil, errors.New("账号已被锁定")
	}

	if !verifyPassword(password, user.PasswordHash) {
		cur := s.store.Get()
		if err := s.userRepo.IncrementLoginAttemptsWithLock(ctx, user.ID, cur.Auth.MaxLoginAttempts, int(cur.Auth.LockoutDuration.Seconds())); err != nil {
			log.Printf("auth: failed to update login attempts: %v", err)
		}
		return nil, errors.New("用户名或密码错误")
	}

	if err := s.userRepo.ResetLoginState(ctx, user.ID, ""); err != nil {
		log.Printf("auth: failed to reset login state: %v", err)
	}

	return user, nil
}

func (s *AuthService) LoginWithInfo(ctx context.Context, username, password, ip, userAgent string) (*User, error) {
	now := time.Now().Format(time.RFC3339)

	user, err := s.Login(ctx, username, password)
	if err != nil {
		evt := LoginEvent{Action: "LOGIN_FAILED", Username: username, IP: ip, UserAgent: userAgent, Success: false, Reason: err.Error(), Time: now}
		s.loginLogger.LogLoginEvent(ctx, evt)
		if s.notifier != nil {
			s.notifier.NotifyLogin(evt)
		}
		return nil, err
	}

	allowed, err := s.CheckIPWhitelist(ctx, user.ID, ip)
	if err != nil {
		return nil, err
	}
	if !allowed {
		evt := LoginEvent{Action: "LOGIN_BLOCKED_IP", Username: username, IP: ip, UserAgent: userAgent, Success: false, Reason: "IP not in whitelist", Time: now}
		s.loginLogger.LogLoginEvent(ctx, evt)
		if s.notifier != nil {
			s.notifier.NotifyLogin(evt)
		}
		return nil, errors.New("login not allowed from this IP")
	}

	expired, err := s.IsAccountExpired(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	if expired {
		s.loginLogger.LogLoginEvent(ctx, LoginEvent{Action: "LOGIN_BLOCKED_EXPIRED", Username: username, IP: ip, UserAgent: userAgent, Success: false, Reason: "account expired", Time: now})
		return nil, errors.New("account has expired")
	}

	if err := s.userRepo.UpdateLastLoginIP(ctx, user.ID, ip); err != nil {
		log.Printf("auth: failed to update last login ip: %v", err)
	}

	evt := LoginEvent{Action: "LOGIN_SUCCESS", Username: username, IP: ip, UserAgent: userAgent, Success: true, Time: now}
	if s.notifier != nil {
		s.notifier.NotifyLogin(evt)
	}

	return user, nil
}

func (s *AuthService) ChangePassword(ctx context.Context, userID int64, oldPassword, newPassword string) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	if user.LockedUntil.Valid && user.LockedUntil.Time.After(time.Now()) {
		return errors.New("account is locked")
	}

	if !verifyPassword(oldPassword, user.PasswordHash) {
		return errors.New("invalid old password")
	}

	if err := s.ValidatePassword(newPassword); err != nil {
		return err
	}

	newHash, err := hashPassword(newPassword)
	if err != nil {
		return err
	}

	return s.userRepo.UpdatePassword(ctx, userID, newHash)
}

func (s *AuthService) GetUserByID(ctx context.Context, id int64) (*User, error) {
	return s.userRepo.GetByID(ctx, id)
}

func (s *AuthService) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	return s.userRepo.GetByUsername(ctx, username)
}

func (s *AuthService) UnlockUser(ctx context.Context, userID int64) error {
	return s.userRepo.ResetLoginState(ctx, userID, "")
}

func (s *AuthService) ForceDisableTOTP(ctx context.Context, userID int64) error {
	return s.totpRepo.DisableTOTP(ctx, userID)
}

func (s *AuthService) ResetPassword(ctx context.Context, userID int64, newPassword string) error {
	if err := s.ValidatePassword(newPassword); err != nil {
		return err
	}

	newHash, err := hashPassword(newPassword)
	if err != nil {
		return err
	}

	if err := s.userRepo.UpdatePassword(ctx, userID, newHash); err != nil {
		return err
	}
	return s.userRepo.SetMustChangePass(ctx, userID, true)
}

func (s *AuthService) ValidatePassword(password string) error {
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	if len(password) > 128 {
		return errors.New("password must be less than 128 characters")
	}

	var hasUpper, hasLower, hasDigit bool
	for _, ch := range password {
		switch {
		case 'A' <= ch && ch <= 'Z':
			hasUpper = true
		case 'a' <= ch && ch <= 'z':
			hasLower = true
		case '0' <= ch && ch <= '9':
			hasDigit = true
		}
	}
	if !hasUpper || !hasLower || !hasDigit {
		return errors.New("password must contain upper, lower case and digit")
	}

	// Check for common weak passwords (after complexity check)
	weakPasswords := []string{
		"password", "12345678", "qwerty123", "admin123",
		"password123", "letmein123", "welcome123",
	}
	lower := strings.ToLower(password)
	if slices.Contains(weakPasswords, lower) {
		return errors.New("password is too common")
	}

	return nil
}

func (s *AuthService) SetAccountExpiry(ctx context.Context, userID int64, expiresAt *time.Time) error {
	return s.userRepo.SetAccountExpiry(ctx, userID, expiresAt)
}

func (s *AuthService) SetIPWhitelist(ctx context.Context, userID int64, whitelist string) error {
	return s.userRepo.SetIPWhitelist(ctx, userID, whitelist)
}

func (s *AuthService) IsTOTPEnabled(ctx context.Context, userID int64) (bool, error) {
	return s.totpRepo.IsTOTPEnabled(ctx, userID)
}

func (s *AuthService) GetTOTPSecret(ctx context.Context, userID int64) (string, error) {
	return s.totpRepo.GetTOTPSecret(ctx, userID)
}

func (s *AuthService) GetIPWhitelist(ctx context.Context, userID int64) (string, error) {
	return s.userRepo.GetIPWhitelist(ctx, userID)
}

func (s *AuthService) CheckIPWhitelist(ctx context.Context, userID int64, ip string) (bool, error) {
	whitelist, err := s.GetIPWhitelist(ctx, userID)
	if err != nil {
		return false, err
	}

	if whitelist == "" {
		return true, nil
	}

	clientIP := net.ParseIP(ip)
	if clientIP == nil {
		return false, nil
	}

	for allowedIP := range strings.SplitSeq(whitelist, ",") {
		allowedIP = strings.TrimSpace(allowedIP)
		if allowedIP == "*" {
			return true, nil
		}
		if strings.Contains(allowedIP, "/") {
			_, cidr, err := net.ParseCIDR(allowedIP)
			if err != nil {
				continue
			}
			if cidr.Contains(clientIP) {
				return true, nil
			}
		} else {
			if allowedIP == ip {
				return true, nil
			}
		}
	}

	return false, nil
}

func (s *AuthService) IsAccountExpired(ctx context.Context, userID int64) (bool, error) {
	expiresAt, err := s.userRepo.GetAccountExpiry(ctx, userID)
	if err != nil {
		return false, err
	}

	if !expiresAt.Valid {
		return false, nil
	}

	return expiresAt.Time.Before(time.Now()), nil
}

// TOTPSetupResult contains the TOTP setup information.
type TOTPSetupResult struct {
	Secret     string `json:"secret"`
	OtpauthURL string `json:"otpauth_url"`
}

// GenerateTOTP generates a new TOTP secret for setup.
func (s *AuthService) GenerateTOTP(userID int64, username string) (*TOTPSetupResult, error) {
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

	return &TOTPSetupResult{
		Secret:     key.Secret(),
		OtpauthURL: key.URL(),
	}, nil
}

// VerifyTOTP verifies a TOTP code against a secret.
func (s *AuthService) VerifyTOTP(secret, code string) bool {
	return totp.Validate(code, secret)
}

// EnableTOTP enables 2FA for a user after verifying the code.
func (s *AuthService) EnableTOTP(ctx context.Context, userID int64, secret, code string) ([]string, error) {
	if !s.VerifyTOTP(secret, code) {
		return nil, errx.Unauthorized("invalid TOTP code")
	}

	backupCodes, err := s.GenerateBackupCodes()
	if err != nil {
		return nil, fmt.Errorf("generate backup codes: %w", err)
	}

	hashedCodes := make([]string, len(backupCodes))
	for i, code := range backupCodes {
		hash, err := hashPassword(code)
		if err != nil {
			return nil, fmt.Errorf("hash backup code: %w", err)
		}
		hashedCodes[i] = hash
	}

	hashedCodesJSON, err := json.Marshal(hashedCodes)
	if err != nil {
		return nil, fmt.Errorf("marshal backup codes: %w", err)
	}

	if err := s.totpRepo.EnableTOTP(ctx, userID, secret, string(hashedCodesJSON)); err != nil {
		return nil, err
	}

	return backupCodes, nil
}

// DisableTOTP disables 2FA for a user after verifying the password.
func (s *AuthService) DisableTOTP(ctx context.Context, userID int64, password string) error {
	passwordHash, err := s.totpRepo.GetPasswordHash(ctx, userID)
	if err != nil {
		return err
	}

	if !verifyPassword(password, passwordHash) {
		return errx.Unauthorized("invalid password")
	}

	return s.totpRepo.DisableTOTP(ctx, userID)
}

// GenerateBackupCodes generates random backup codes.
func (s *AuthService) GenerateBackupCodes() ([]string, error) {
	codes := make([]string, backupCodeCount)
	for i := range backupCodeCount {
		code, err := generateRandomCode(backupCodeLength)
		if err != nil {
			return nil, fmt.Errorf("generate random code: %w", err)
		}
		codes[i] = code
	}
	return codes, nil
}

// VerifyBackupCode verifies and consumes a backup code.
func (s *AuthService) VerifyBackupCode(ctx context.Context, userID int64, code string) (bool, error) {
	backupCodesJSON, err := s.totpRepo.GetBackupCodes(ctx, userID)
	if err != nil {
		return false, err
	}

	var hashedCodes []string
	if err := json.Unmarshal([]byte(backupCodesJSON), &hashedCodes); err != nil {
		return false, fmt.Errorf("parse backup codes: %w", err)
	}

	for i, hashedCode := range hashedCodes {
		if verifyPassword(code, hashedCode) {
			hashedCodes = append(hashedCodes[:i], hashedCodes[i+1:]...)
			newJSON, err := json.Marshal(hashedCodes)
			if err != nil {
				return false, fmt.Errorf("marshal updated backup codes: %w", err)
			}

			if err := s.totpRepo.UpdateBackupCodes(ctx, userID, string(newJSON)); err != nil {
				return false, err
			}

			return true, nil
		}
	}

	return false, nil
}

// GetPendingSecret gets the pending TOTP secret for a user (during setup).
func (s *AuthService) GetPendingSecret(ctx context.Context, userID int64) (string, error) {
	return s.totpRepo.GetPendingSecret(ctx, userID)
}

// StorePendingSecret stores a TOTP secret temporarily during setup.
func (s *AuthService) StorePendingSecret(ctx context.Context, userID int64, secret string) error {
	return s.totpRepo.StorePendingSecret(ctx, userID, secret)
}

// Helper functions

func generateRandomCode(length int) (string, error) {
	const digits = "0123456789"
	code := make([]byte, length)
	for i := range length {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		if err != nil {
			return "", err
		}
		code[i] = digits[num.Int64()]
	}
	return string(code), nil
}

func hashPassword(password string) (string, error) {
	passwordBytes := []byte(password)
	if len(passwordBytes) > 72 {
		hash := sha256.Sum256(passwordBytes)
		passwordBytes = []byte(hex.EncodeToString(hash[:]))
	}
	hash, err := bcrypt.GenerateFromPassword(passwordBytes, bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func verifyPassword(password, hash string) bool {
	passwordBytes := []byte(password)
	if len(passwordBytes) > 72 {
		h := sha256.Sum256(passwordBytes)
		passwordBytes = []byte(hex.EncodeToString(h[:]))
	}
	err := bcrypt.CompareHashAndPassword([]byte(hash), passwordBytes)
	return err == nil
}
