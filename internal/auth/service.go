package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"github.com/skip2/go-qrcode"
	"golang.org/x/crypto/bcrypt"
)

const (
	TokenExpiry          = 24 * time.Hour
	CacheCleanupInterval = 5 * time.Minute
	InvalidationExpiry   = 365 * TokenExpiry

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

type tokenCache struct {
	blacklisted   sync.Map
	invalidations sync.Map
}

type AuthService struct {
	userRepo        UserRepo
	tokenRepo       TokenBlacklistRepo
	activityRepo    ActivityRepo
	totpRepo        TOTPRepo
	maxAttempts     int
	lockoutDuration time.Duration
	cache           tokenCache
	notifier        LoginNotifier
	done            chan struct{}
}

func NewAuthService(maxAttempts int, lockoutDuration time.Duration) *AuthService {
	s := &AuthService{
		maxAttempts:     maxAttempts,
		lockoutDuration: lockoutDuration,
		done:            make(chan struct{}),
	}
	go s.cacheCleanupLoop()
	go s.tokenBlacklistCleanupLoop()
	return s
}

func (s *AuthService) Close() {
	select {
	case <-s.done:
	default:
		close(s.done)
	}
}

func (s *AuthService) SetRepositories(userRepo UserRepo, tokenRepo TokenBlacklistRepo, activityRepo ActivityRepo, totpRepo TOTPRepo) {
	s.userRepo = userRepo
	s.tokenRepo = tokenRepo
	s.activityRepo = activityRepo
	s.totpRepo = totpRepo
}

func (s *AuthService) SetNotifyService(notifier LoginNotifier) {
	s.notifier = notifier
}

func (s *AuthService) tokenBlacklistCleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := s.CleanupExpiredTokens(context.Background()); err != nil {
				log.Printf("auth: failed to cleanup expired tokens: %v", err)
			}
		case <-s.done:
			return
		}
	}
}

func (s *AuthService) cacheCleanupLoop() {
	ticker := time.NewTicker(CacheCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			now := time.Now()
			s.cache.blacklisted.Range(func(key, value any) bool {
				if t, ok := value.(time.Time); ok && t.Before(now) {
					s.cache.blacklisted.Delete(key)
				}
				return true
			})
			s.cache.invalidations.Range(func(key, value any) bool {
				if t, ok := value.(time.Time); ok && t.Add(365*TokenExpiry).Before(now) {
					s.cache.invalidations.Delete(key)
				}
				return true
			})
		case <-s.done:
			return
		}
	}
}

func (s *AuthService) InitDefaultAdmin(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	_, total, err := s.userRepo.List(ctx, 0, 1)
	if err != nil {
		return err
	}
	if total == 0 {
		password := generateRandomPassword(16)
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		if err := s.userRepo.Create(ctx, &User{
			Username:       "admin",
			PasswordHash:   string(hash),
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
	if ctx == nil {
		ctx = context.Background()
	}

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

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		if err := s.userRepo.IncrementLoginAttemptsWithLock(ctx, user.ID, s.maxAttempts, int(s.lockoutDuration.Seconds())); err != nil {
			log.Printf("auth: failed to update login attempts: %v", err)
		}
		return nil, errors.New("用户名或密码错误")
	}

	s.userRepo.ResetLoginState(ctx, user.ID, "")

	return user, nil
}

func (s *AuthService) LoginWithInfo(ctx context.Context, username, password, ip, userAgent string) (*User, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	user, err := s.Login(ctx, username, password)
	if err != nil {
		s.LogUserActivity(ctx, 0, username, "LOGIN_FAILED", ip, userAgent)
		if s.notifier != nil {
			s.notifier.NotifyLogin(LoginEvent{
				Username:  username,
				IP:        ip,
				UserAgent: userAgent,
				Time:      time.Now().Format(time.RFC3339),
				Success:   false,
				Reason:    err.Error(),
			})
		}
		return nil, err
	}

	allowed, err := s.CheckIPWhitelist(ctx, user.ID, ip)
	if err != nil {
		return nil, err
	}
	if !allowed {
		s.LogUserActivity(ctx, user.ID, username, "LOGIN_BLOCKED_IP", ip, userAgent)
		if s.notifier != nil {
			s.notifier.NotifyLogin(LoginEvent{
				Username:  username,
				IP:        ip,
				UserAgent: userAgent,
				Time:      time.Now().Format(time.RFC3339),
				Success:   false,
				Reason:    "IP not in whitelist",
			})
		}
		return nil, errors.New("login not allowed from this IP")
	}

	expired, err := s.IsAccountExpired(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	if expired {
		s.LogUserActivity(ctx, user.ID, username, "LOGIN_BLOCKED_EXPIRED", ip, userAgent)
		return nil, errors.New("account has expired")
	}

	s.userRepo.UpdateLastLoginIP(ctx, user.ID, ip)

	s.LogUserActivity(ctx, user.ID, username, "LOGIN_SUCCESS", ip, userAgent)

	if s.notifier != nil {
		s.notifier.NotifyLogin(LoginEvent{
			Username:  username,
			IP:        ip,
			UserAgent: userAgent,
			Time:      time.Now().Format(time.RFC3339),
			Success:   true,
		})
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

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(oldPassword)); err != nil {
		return errors.New("invalid old password")
	}

	if err := s.ValidatePassword(newPassword); err != nil {
		return err
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	return s.userRepo.UpdatePassword(ctx, userID, string(newHash))
}

func (s *AuthService) GetUserByID(ctx context.Context, id int64) (*User, error) {
	return s.userRepo.GetByID(ctx, id)
}

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func (s *AuthService) AddTokenToBlacklist(ctx context.Context, userID int64, token string, expiresAt time.Time) error {
	if ctx == nil {
		ctx = context.Background()
	}
	tokenHash := hashToken(token)
	err := s.tokenRepo.Add(ctx, userID, tokenHash, expiresAt)
	if err == nil {
		s.cache.blacklisted.Store(tokenHash, expiresAt)
	}
	return err
}

func (s *AuthService) IsTokenBlacklisted(ctx context.Context, token string) (bool, error) {
	tokenHash := hashToken(token)
	if v, ok := s.cache.blacklisted.Load(tokenHash); ok {
		if t, ok := v.(time.Time); ok && t.After(time.Now()) {
			return true, nil
		}
		s.cache.blacklisted.Delete(tokenHash)
	}
	return s.tokenRepo.IsBlacklisted(ctx, tokenHash)
}

func (s *AuthService) InvalidateAllUserTokens(ctx context.Context, userID int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now()
	if err := s.tokenRepo.AddUserInvalidation(ctx, userID); err != nil {
		return err
	}
	s.cache.invalidations.Store(userID, now)
	return nil
}

func (s *AuthService) IsUserTokenInvalidated(ctx context.Context, userID int64, issuedAt time.Time) (bool, error) {
	if v, ok := s.cache.invalidations.Load(userID); ok {
		if t, ok := v.(time.Time); ok {
			return issuedAt.Before(t), nil
		}
	}
	invalidated, err := s.tokenRepo.IsUserInvalidated(ctx, userID, issuedAt)
	if err != nil {
		log.Printf("auth: error checking token invalidation: %v", err)
		return false, nil
	}
	return invalidated, nil
}

func (s *AuthService) CleanupExpiredTokens(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.tokenRepo.Clean(ctx)
}

func (s *AuthService) ResetPassword(ctx context.Context, userID int64, newPassword string) error {
	if err := s.ValidatePassword(newPassword); err != nil {
		return err
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	if err := s.userRepo.UpdatePassword(ctx, userID, string(newHash)); err != nil {
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

	// Check for common weak passwords
	weakPasswords := []string{
		"password", "12345678", "qwerty123", "admin123",
		"password123", "letmein123", "welcome123",
	}
	lower := strings.ToLower(password)
	for _, weak := range weakPasswords {
		if lower == weak {
			return errors.New("password is too common")
		}
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
	if !(hasUpper && hasLower && hasDigit) {
		return errors.New("password must contain upper, lower case and digit")
	}
	return nil
}

func (s *AuthService) LogUserActivity(ctx context.Context, userID int64, username, action, ip, userAgent string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.activityRepo.Log(ctx, &UserActivity{
		UserID:    userID,
		Username:  username,
		Action:    action,
		IP:        ip,
		UserAgent: userAgent,
	})
}

func (s *AuthService) GetUserActivities(ctx context.Context, userID int64, limit int) ([]UserActivity, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.activityRepo.GetByUserID(ctx, userID, limit)
}

func (s *AuthService) GetAllActivities(ctx context.Context, limit int) ([]UserActivity, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.activityRepo.GetAll(ctx, limit)
}

func (s *AuthService) SetAccountExpiry(ctx context.Context, userID int64, expiresAt *time.Time) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.userRepo.SetAccountExpiry(ctx, userID, expiresAt)
}

func (s *AuthService) SetIPWhitelist(ctx context.Context, userID int64, whitelist string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.userRepo.SetIPWhitelist(ctx, userID, whitelist)
}

func (s *AuthService) IsTOTPEnabled(ctx context.Context, userID int64) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.totpRepo.IsTOTPEnabled(ctx, userID)
}

func (s *AuthService) GetTOTPSecret(ctx context.Context, userID int64) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.totpRepo.GetTOTPSecret(ctx, userID)
}

func (s *AuthService) GetIPWhitelist(ctx context.Context, userID int64) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.userRepo.GetIPWhitelist(ctx, userID)
}

func (s *AuthService) CheckIPWhitelist(ctx context.Context, userID int64, ip string) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
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

	for _, allowedIP := range strings.Split(whitelist, ",") {
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
	if ctx == nil {
		ctx = context.Background()
	}
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
	Secret       string `json:"secret"`
	OtpauthURL   string `json:"otpauth_url"`
	QRCodeBase64 string `json:"qr_code_base64"`
}

// GenerateTOTP generates a new TOTP secret and QR code for setup.
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

	qrCode, err := qrcode.Encode(key.URL(), qrcode.Medium, 256)
	if err != nil {
		return nil, fmt.Errorf("generate QR code: %w", err)
	}

	qrCodeBase64 := fmt.Sprintf("data:image/png;base64,%s", base64Encode(qrCode))

	return &TOTPSetupResult{
		Secret:       key.Secret(),
		OtpauthURL:   key.URL(),
		QRCodeBase64: qrCodeBase64,
	}, nil
}

// VerifyTOTP verifies a TOTP code against a secret.
func (s *AuthService) VerifyTOTP(secret, code string) bool {
	return totp.Validate(code, secret)
}

// EnableTOTP enables 2FA for a user after verifying the code.
func (s *AuthService) EnableTOTP(ctx context.Context, userID int64, secret, code string) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !s.VerifyTOTP(secret, code) {
		return nil, fmt.Errorf("invalid TOTP code")
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
	if ctx == nil {
		ctx = context.Background()
	}
	passwordHash, err := s.totpRepo.GetPasswordHash(ctx, userID)
	if err != nil {
		return err
	}

	if !verifyPassword(password, passwordHash) {
		return fmt.Errorf("invalid password")
	}

	return s.totpRepo.DisableTOTP(ctx, userID)
}

// GenerateBackupCodes generates random backup codes.
func (s *AuthService) GenerateBackupCodes() ([]string, error) {
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

// VerifyBackupCode verifies and consumes a backup code.
func (s *AuthService) VerifyBackupCode(ctx context.Context, userID int64, code string) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
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
	if ctx == nil {
		ctx = context.Background()
	}
	return s.totpRepo.GetPendingSecret(ctx, userID)
}

// StorePendingSecret stores a TOTP secret temporarily during setup.
func (s *AuthService) StorePendingSecret(ctx context.Context, userID int64, secret string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.totpRepo.StorePendingSecret(ctx, userID, secret)
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
