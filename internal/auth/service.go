package auth

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
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
	// ponytail: TOTP 设置期的待启用密钥放进程内存，重启即弃（本就不应跨重启保留）。
	pendingSecrets sync.Map // userID(int64) -> secret(string)
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
		_, _ = os.Stdout.WriteString("=================================================\n")
		_, _ = os.Stdout.WriteString("EasyServer 初次启动 - 管理员账号信息\n")
		_, _ = os.Stdout.WriteString("=================================================\n")
		_, _ = os.Stdout.WriteString("用户名: admin\n")
		_, _ = os.Stdout.WriteString("密码:   " + password + "\n")
		_, _ = os.Stdout.WriteString("=================================================\n")
		_, _ = os.Stdout.WriteString("请登录后立即修改密码！\n")
		_, _ = os.Stdout.WriteString("=================================================\n")
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
	return s.LoginWithInfo(ctx, username, password, "", "")
}

func (s *AuthService) LoginWithInfo(ctx context.Context, username, password, ip, userAgent string) (*User, error) {
	now := time.Now().Format(time.RFC3339)

	user, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		evt := LoginEvent{Action: "LOGIN_FAILED", Username: username, IP: ip, UserAgent: userAgent, Success: false, Reason: "用户名或密码错误", Time: now}
		if s.loginLogger != nil {
			s.loginLogger.LogLoginEvent(ctx, evt)
		}
		if s.notifier != nil {
			s.notifier.NotifyLogin(evt)
		}
		return nil, errors.New("用户名或密码错误")
	}

	if user.LockedUntil.Valid && user.LockedUntil.Time.After(time.Now()) {
		return nil, errors.New("账号已被锁定")
	}

	if !verifyPassword(password, user.PasswordHash) {
		cur := s.store.Get()
		justLocked, err := s.userRepo.IncrementLoginAttempts(ctx, user.ID, cur.Auth.MaxLoginAttempts, int(cur.Auth.LockoutDuration.Seconds()))
		if err != nil {
			log.Printf("auth: failed to update login attempts: %v", err)
		}
		evt := LoginEvent{Action: "LOGIN_FAILED", Username: username, IP: ip, UserAgent: userAgent, Success: false, Reason: "用户名或密码错误", Time: now}
		if justLocked {
			evt.Reason = "账号已被锁定"
		}
		if s.loginLogger != nil {
			s.loginLogger.LogLoginEvent(ctx, evt)
		}
		if s.notifier != nil {
			s.notifier.NotifyLogin(evt)
		}
		return nil, errors.New("用户名或密码错误")
	}

	if err := s.userRepo.ResetLoginState(ctx, user.ID, ip); err != nil {
		log.Printf("auth: failed to reset login state: %v", err)
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

	if oldPassword == newPassword {
		return errors.New("新密码不能与旧密码相同")
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

func (s *AuthService) ChangeUsername(ctx context.Context, userID int64, newUsername, password string) error {
	newUsername = strings.TrimSpace(newUsername)
	if len(newUsername) < 3 || len(newUsername) > 32 {
		return errors.New("用户名长度必须在 3 到 32 个字符之间")
	}

	for _, ch := range newUsername {
		isValid := (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-'
		if !isValid {
			return errors.New("用户名只能包含字母、数字、下划线或短横线")
		}
	}

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	if user.LockedUntil.Valid && user.LockedUntil.Time.After(time.Now()) {
		return errors.New("账户已锁定")
	}

	if !verifyPassword(password, user.PasswordHash) {
		return errors.New("当前密码不正确")
	}

	if user.Username == newUsername {
		return errors.New("新用户名不能与当前用户名相同")
	}

	existing, err := s.userRepo.GetByUsername(ctx, newUsername)
	if err == nil && existing != nil && existing.ID != userID {
		return errors.New("该用户名已存在")
	}

	return s.userRepo.UpdateUsername(ctx, userID, newUsername)
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

	return s.userRepo.UpdatePassword(ctx, userID, newHash)
}

func (s *AuthService) ValidatePassword(password string) error {
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	if len(password) > 72 {
		return errors.New("password must be less than 72 characters")
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

func (s *AuthService) IsTOTPEnabled(ctx context.Context, userID int64) (bool, error) {
	return s.totpRepo.IsTOTPEnabled(ctx, userID)
}

func (s *AuthService) GetTOTPSecret(ctx context.Context, userID int64) (string, error) {
	return s.totpRepo.GetTOTPSecret(ctx, userID)
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
	v, ok := s.pendingSecrets.Load(userID)
	if !ok {
		return "", errors.New("no pending TOTP secret found")
	}
	return v.(string), nil
}

// StorePendingSecret stores a TOTP secret temporarily during setup.
func (s *AuthService) StorePendingSecret(ctx context.Context, userID int64, secret string) error {
	s.pendingSecrets.Store(userID, secret)
	return nil
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
	if len(password) > 72 {
		return "", errors.New("password exceeds maximum length of 72 bytes")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func verifyPassword(password, hash string) bool {
	if len(password) > 72 {
		return false
	}
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
