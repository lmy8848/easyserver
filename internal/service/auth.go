package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"easyserver/internal/model"
	"easyserver/internal/repository"

	"golang.org/x/crypto/bcrypt"
)

// 认证相关常量
const (
	TokenExpiry          = 24 * time.Hour        // JWT token 过期时间
	CacheCleanupInterval = 5 * time.Minute       // 缓存清理间隔
	InvalidationExpiry   = 365 * TokenExpiry     // 失效标记过期时间
)

// tokenCache provides in-memory caching for token blacklist lookups.
type tokenCache struct {
	blacklisted   sync.Map // token (string) -> expiresAt (time.Time)
	invalidations sync.Map // userID (int64) -> invalidatedAt (time.Time)
}

type AuthService struct {
	db              *sql.DB
	userRepo        repository.UserRepository
	tokenRepo       repository.TokenBlacklistRepository
	maxAttempts     int
	lockoutDuration time.Duration
	cache           tokenCache
	notifyService   *NotifyService
}

func NewAuthService(db *sql.DB, maxAttempts int, lockoutDuration time.Duration) *AuthService {
	s := &AuthService{
		db:              db,
		maxAttempts:     maxAttempts,
		lockoutDuration: lockoutDuration,
	}
	go s.cacheCleanupLoop()
	go s.tokenBlacklistCleanupLoop()
	return s
}

// SetRepositories sets the repository implementations for the auth service
func (s *AuthService) SetRepositories(userRepo repository.UserRepository, tokenRepo repository.TokenBlacklistRepository) {
	s.userRepo = userRepo
	s.tokenRepo = tokenRepo
}

// SetNotifyService sets the notification service for login alerts
func (s *AuthService) SetNotifyService(notifyService *NotifyService) {
	s.notifyService = notifyService
}

// tokenBlacklistCleanupLoop removes expired tokens from the database every hour.
func (s *AuthService) tokenBlacklistCleanupLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		if err := s.CleanupExpiredTokens(context.Background()); err != nil {
			log.Printf("auth: failed to cleanup expired tokens: %v", err)
		}
	}
}

// cacheCleanupLoop removes expired entries from the in-memory cache every 5 minutes.
func (s *AuthService) cacheCleanupLoop() {
	ticker := time.NewTicker(CacheCleanupInterval)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		s.cache.blacklisted.Range(func(key, value any) bool {
			if t, ok := value.(time.Time); ok && t.Before(now) {
				s.cache.blacklisted.Delete(key)
			}
			return true
		})
		s.cache.invalidations.Range(func(key, value any) bool {
			if t, ok := value.(time.Time); ok && t.Add(365 * TokenExpiry).Before(now) {
				s.cache.invalidations.Delete(key)
			}
			return true
		})
	}
}

func (s *AuthService) InitDefaultAdmin(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	if s.userRepo != nil {
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
			if err := s.userRepo.Create(ctx, &model.User{
				Username:       "admin",
				PasswordHash:   string(hash),
				Role:           model.RoleAdmin,
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
		}
		return nil
	}

	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 {
		// Generate random 16-character password
		password := generateRandomPassword(16)
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		_, err = s.db.ExecContext(ctx,
			"INSERT INTO users (username, password_hash, role, must_change_pass) VALUES (?, ?, ?, ?)",
			"admin", string(hash), model.RoleAdmin, true,
		)
		if err != nil {
			return err
		}

		// Print the random password to stdout (only on first start)
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

// generateRandomPassword generates a cryptographically secure random password
// of the specified length using rejection sampling to avoid modulo bias.
func generateRandomPassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	// charsetLen must be a power-of-2 mask boundary; use rejection sampling
	// to ensure a uniform distribution over the charset.
	charsetLen := byte(len(charset))
	// limit is the largest multiple of charsetLen <= 256
	limit := 256 - (256 % int(charsetLen))
	password := make([]byte, length)
	for i := range password {
		for {
			b := make([]byte, 1)
			if _, err := rand.Read(b); err != nil {
				continue
			}
			// Reject values that would introduce modulo bias
			if int(b[0]) >= limit {
				continue
			}
			password[i] = charset[int(b[0])%int(charsetLen)]
			break
		}
	}
	return string(password)
}

func (s *AuthService) Login(ctx context.Context, username, password string) (*model.User, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	var user *model.User
	if s.userRepo != nil {
		var err error
		user, err = s.userRepo.GetByUsername(ctx, username)
		if err != nil {
			return nil, errors.New("invalid credentials")
		}
	} else {
		user = &model.User{}
		var mustChangePass int
		err := s.db.QueryRowContext(ctx,
			"SELECT id, username, password_hash, role, must_change_pass, last_login, login_attempts, locked_until FROM users WHERE username = ?",
			username,
		).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &mustChangePass, &user.LastLogin, &user.LoginAttempts, &user.LockedUntil)
		user.MustChangePass = mustChangePass != 0

		if err == sql.ErrNoRows {
			return nil, errors.New("invalid credentials")
		}
		if err != nil {
			return nil, err
		}
	}

	if user.LockedUntil.Valid && user.LockedUntil.Time.After(time.Now()) {
		if _, err := s.db.ExecContext(ctx, "UPDATE users SET login_attempts = login_attempts + 1 WHERE id = ?", user.ID); err != nil {
			log.Printf("auth: failed to increment login attempts: %v", err)
		}
		return nil, errors.New("invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		// Atomic update: increment attempts and lock if threshold reached
		if _, err := s.db.ExecContext(ctx, `UPDATE users SET
			login_attempts = login_attempts + 1,
			locked_until = CASE
				WHEN login_attempts + 1 >= ? THEN datetime('now', ?)
				ELSE locked_until
			END
			WHERE id = ?`, s.maxAttempts, fmt.Sprintf("+%d seconds", int(s.lockoutDuration.Seconds())), user.ID); err != nil {
			log.Printf("auth: failed to update login attempts: %v", err)
		}
		return nil, errors.New("invalid credentials")
	}

	// Reset login attempts on success and update last login IP
	s.db.ExecContext(ctx, "UPDATE users SET login_attempts = 0, locked_until = NULL, last_login = CURRENT_TIMESTAMP, last_login_ip = ? WHERE id = ?", "", user.ID)

	return user, nil
}

// LoginWithInfo logs in a user and records the login activity
func (s *AuthService) LoginWithInfo(ctx context.Context, username, password, ip, userAgent string) (*model.User, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	user, err := s.Login(ctx, username, password)
	if err != nil {
		// Log failed login attempt
		s.LogUserActivity(ctx, 0, username, "LOGIN_FAILED", ip, userAgent)
		// Send notification for failed login
		if s.notifyService != nil {
			s.notifyService.NotifyLogin(LoginEvent{
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

	// Check IP whitelist
	allowed, err := s.CheckIPWhitelist(ctx, user.ID, ip)
	if err != nil {
		return nil, err
	}
	if !allowed {
		s.LogUserActivity(ctx, user.ID, username, "LOGIN_BLOCKED_IP", ip, userAgent)
		// Send notification for IP block
		if s.notifyService != nil {
			s.notifyService.NotifyLogin(LoginEvent{
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

	// Check account expiry
	expired, err := s.IsAccountExpired(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	if expired {
		s.LogUserActivity(ctx, user.ID, username, "LOGIN_BLOCKED_EXPIRED", ip, userAgent)
		return nil, errors.New("account has expired")
	}

	// Update last login IP
	s.db.ExecContext(ctx, "UPDATE users SET last_login_ip = ? WHERE id = ?", ip, user.ID)

	// Log successful login
	s.LogUserActivity(ctx, user.ID, username, "LOGIN_SUCCESS", ip, userAgent)

	// Send notification for successful login
	if s.notifyService != nil {
		s.notifyService.NotifyLogin(LoginEvent{
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
	var hash string
	var lockedUntil sql.NullTime

	if s.userRepo != nil {
		user, err := s.userRepo.GetByID(ctx, userID)
		if err != nil {
			return err
		}
		hash = user.PasswordHash
		lockedUntil = user.LockedUntil
	} else {
		err := s.db.QueryRowContext(ctx, "SELECT password_hash, locked_until FROM users WHERE id = ?", userID).Scan(&hash, &lockedUntil)
		if err != nil {
			return err
		}
	}

	// Check if account is locked
	if lockedUntil.Valid && lockedUntil.Time.After(time.Now()) {
		return errors.New("account is locked")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(oldPassword)); err != nil {
		return errors.New("invalid old password")
	}

	// Validate password strength
	if err := s.ValidatePassword(newPassword); err != nil {
		return err
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	if s.userRepo != nil {
		return s.userRepo.UpdatePassword(ctx, userID, string(newHash))
	}

	_, err = s.db.ExecContext(ctx, "UPDATE users SET password_hash = ?, must_change_pass = 0, updated_at = CURRENT_TIMESTAMP WHERE id = ?", string(newHash), userID)
	return err
}

func (s *AuthService) GetUserByID(ctx context.Context, id int64) (*model.User, error) {
	if s.userRepo != nil {
		return s.userRepo.GetByID(ctx, id)
	}
	user := &model.User{}
	var lastLoginStr, createdAtStr sql.NullString
	var mustChangePass int
	err := s.db.QueryRowContext(ctx,
		"SELECT id, username, role, must_change_pass, last_login, created_at FROM users WHERE id = ?", id,
	).Scan(&user.ID, &user.Username, &user.Role, &mustChangePass, &lastLoginStr, &createdAtStr)
	if err != nil {
		return nil, err
	}
	user.MustChangePass = mustChangePass != 0

	// Parse last_login
	if lastLoginStr.Valid && lastLoginStr.String != "" {
		t, err := time.Parse("2006-01-02 15:04:05", lastLoginStr.String)
		if err == nil {
			user.LastLogin = sql.NullTime{Time: t, Valid: true}
		}
	}

	// Parse created_at
	if createdAtStr.Valid && createdAtStr.String != "" {
		t, err := time.Parse("2006-01-02 15:04:05", createdAtStr.String)
		if err == nil {
			user.CreatedAt = t
		} else {
			user.CreatedAt = time.Now()
		}
	} else {
		user.CreatedAt = time.Now()
	}

	return user, nil
}

// hashToken returns the SHA-256 hex digest of a token.
// Storing hashes instead of full JWTs reduces exposure if the database is compromised.
func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// AddTokenToBlacklist adds a token to the blacklist (stores SHA-256 hash)
func (s *AuthService) AddTokenToBlacklist(ctx context.Context, userID int64, token string, expiresAt time.Time) error {
	if ctx == nil {
		ctx = context.Background()
	}
	tokenHash := hashToken(token)
	var err error
	if s.tokenRepo != nil {
		err = s.tokenRepo.Add(ctx, userID, tokenHash, expiresAt)
	} else {
		_, err = s.db.ExecContext(ctx,
			"INSERT INTO token_blacklist (user_id, token, expires_at) VALUES (?, ?, ?)",
			userID, tokenHash, expiresAt,
		)
	}
	if err == nil {
		s.cache.blacklisted.Store(tokenHash, expiresAt)
	}
	return err
}

// IsTokenBlacklisted checks if a token is blacklisted (looks up by SHA-256 hash)
func (s *AuthService) IsTokenBlacklisted(ctx context.Context, token string) (bool, error) {
	tokenHash := hashToken(token)
	if v, ok := s.cache.blacklisted.Load(tokenHash); ok {
		if t, ok := v.(time.Time); ok && t.After(time.Now()) {
			return true, nil
		}
		s.cache.blacklisted.Delete(tokenHash)
	}
	if s.tokenRepo != nil {
		return s.tokenRepo.IsBlacklisted(ctx, tokenHash)
	}
	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM token_blacklist WHERE token = ? AND expires_at > ?",
		tokenHash, time.Now(),
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// InvalidateAllUserTokens invalidates all tokens for a user
// Stores the invalidation timestamp so tokens issued before this time are rejected
func (s *AuthService) InvalidateAllUserTokens(ctx context.Context, userID int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now()
	if s.tokenRepo != nil {
		if err := s.tokenRepo.AddUserInvalidation(ctx, userID); err != nil {
			return err
		}
		s.cache.invalidations.Store(userID, now)
		return nil
	}
	// Fallback path: use a transaction to avoid the DELETE-INSERT race window
	tx, txErr := s.db.BeginTx(ctx, nil)
	if txErr != nil {
		return txErr
	}
	defer tx.Rollback()
	// Delete any existing markers for this user first
	tx.ExecContext(ctx, "DELETE FROM token_blacklist WHERE user_id = ? AND token LIKE 'user_%_all'", userID)

	// Add a marker with the current time as the invalidation point
	// expires_at is set far in the future to keep the marker active
	_, err := tx.Exec(
		"INSERT INTO token_blacklist (user_id, token, expires_at) VALUES (?, ?, ?)",
		userID, fmt.Sprintf("user_%d_all", userID), now.Add(365*24*time.Hour),
	)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.cache.invalidations.Store(userID, now)
	return nil
}

// IsUserTokenInvalidated checks if a user's tokens have been invalidated
// Returns true if the token was issued before the invalidation time
// On database errors, returns false (not invalidated) to avoid blocking requests
func (s *AuthService) IsUserTokenInvalidated(ctx context.Context, userID int64, issuedAt time.Time) (bool, error) {
	if v, ok := s.cache.invalidations.Load(userID); ok {
		if t, ok := v.(time.Time); ok {
			return issuedAt.Before(t), nil
		}
	}
	if s.tokenRepo != nil {
		invalidated, err := s.tokenRepo.IsUserInvalidated(ctx, userID, issuedAt)
		if err != nil {
			log.Printf("auth: error checking token invalidation: %v", err)
			return false, nil
		}
		return invalidated, nil
	}
	var createdAt time.Time
	// Get the most recent invalidation marker for this user
	err := s.db.QueryRow(
		"SELECT created_at FROM token_blacklist WHERE user_id = ? AND token LIKE 'user_%_all' AND expires_at > datetime('now') ORDER BY created_at DESC LIMIT 1",
		userID,
	).Scan(&createdAt)
	if err == sql.ErrNoRows {
		// No invalidation marker found
		return false, nil
	}
	if err != nil {
		// On database error, log and return false (not invalidated)
		// This avoids blocking requests due to transient database issues
		log.Printf("auth: error checking token invalidation: %v", err)
		return false, nil
	}

	// Warm the cache for subsequent requests
	s.cache.invalidations.Store(userID, createdAt)

	// Token is invalidated if it was issued before the invalidation marker
	return issuedAt.Before(createdAt), nil
}

// CleanupExpiredTokens removes expired tokens from blacklist
func (s *AuthService) CleanupExpiredTokens(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if s.tokenRepo != nil {
		return s.tokenRepo.Clean(ctx)
	}
	_, err := s.db.ExecContext(ctx, "DELETE FROM token_blacklist WHERE expires_at < ?", time.Now())
	return err
}

// ResetPassword resets a user's password (admin only)
func (s *AuthService) ResetPassword(ctx context.Context, userID int64, newPassword string) error {
	// Validate password strength
	if err := s.ValidatePassword(newPassword); err != nil {
		return err
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	if s.userRepo != nil {
		if err := s.userRepo.UpdatePassword(ctx, userID, string(newHash)); err != nil {
			return err
		}
		return s.userRepo.SetMustChangePass(ctx, userID, true)
	}

	_, err = s.db.Exec(
		"UPDATE users SET password_hash = ?, must_change_pass = 1, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		string(newHash), userID,
	)
	return err
}

// ValidatePassword validates password strength.
// Rules mirror middleware.DefaultPasswordStrength: min 8 chars, upper + lower + digit.
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
	if !(hasUpper && hasLower && hasDigit) {
		return errors.New("password must contain upper, lower case and digit")
	}
	return nil
}

// LogUserActivity logs a user activity
func (s *AuthService) LogUserActivity(ctx context.Context, userID int64, username, action, ip, userAgent string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO user_activities (user_id, username, action, ip, user_agent) VALUES (?, ?, ?, ?, ?)",
		userID, username, action, ip, userAgent,
	)
	return err
}

// GetUserActivities returns user activity log
func (s *AuthService) GetUserActivities(ctx context.Context, userID int64, limit int) ([]model.UserActivity, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	rows, err := s.db.QueryContext(ctx,
		"SELECT id, user_id, username, action, ip, user_agent, created_at FROM user_activities WHERE user_id = ? ORDER BY id DESC LIMIT ?",
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activities []model.UserActivity
	for rows.Next() {
		var a model.UserActivity
		if err := rows.Scan(&a.ID, &a.UserID, &a.Username, &a.Action, &a.IP, &a.UserAgent, &a.CreatedAt); err != nil {
			continue
		}
		activities = append(activities, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return activities, nil
}

// GetAllActivities returns all user activities
func (s *AuthService) GetAllActivities(ctx context.Context, limit int) ([]model.UserActivity, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx,
		"SELECT id, user_id, username, action, ip, user_agent, created_at FROM user_activities ORDER BY id DESC LIMIT ?",
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var activities []model.UserActivity
	for rows.Next() {
		var a model.UserActivity
		if err := rows.Scan(&a.ID, &a.UserID, &a.Username, &a.Action, &a.IP, &a.UserAgent, &a.CreatedAt); err != nil {
			continue
		}
		activities = append(activities, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return activities, nil
}

// SetAccountExpiry sets account expiration date
func (s *AuthService) SetAccountExpiry(ctx context.Context, userID int64, expiresAt *time.Time) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if expiresAt == nil {
		_, err := s.db.ExecContext(ctx, "UPDATE users SET expires_at = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ?", userID)
		return err
	}
	_, err := s.db.ExecContext(ctx, "UPDATE users SET expires_at = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", expiresAt, userID)
	return err
}

// SetIPWhitelist sets IP whitelist for a user
func (s *AuthService) SetIPWhitelist(ctx context.Context, userID int64, whitelist string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	_, err := s.db.ExecContext(ctx, "UPDATE users SET ip_whitelist = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", whitelist, userID)
	return err
}

// IsTOTPEnabled checks if 2FA is enabled for a user
func (s *AuthService) IsTOTPEnabled(ctx context.Context, userID int64) (bool, error) {
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
func (s *AuthService) GetTOTPSecret(ctx context.Context, userID int64) (string, error) {
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

// GetIPWhitelist returns the raw IP whitelist string for a user (comma-separated,
// empty means allow-all). Used by middleware to fetch the whitelist without
// touching the database directly.
func (s *AuthService) GetIPWhitelist(ctx context.Context, userID int64) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var whitelist string
	err := s.db.QueryRowContext(ctx, "SELECT COALESCE(ip_whitelist, '') FROM users WHERE id = ?", userID).Scan(&whitelist)
	return whitelist, err
}

// CheckIPWhitelist checks if an IP is in the user's whitelist
func (s *AuthService) CheckIPWhitelist(ctx context.Context, userID int64, ip string) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	whitelist, err := s.GetIPWhitelist(ctx, userID)
	if err != nil {
		return false, err
	}

	// If whitelist is empty, allow all IPs
	if whitelist == "" {
		return true, nil
	}

	// Parse the client IP
	clientIP := net.ParseIP(ip)
	if clientIP == nil {
		return false, nil
	}

	// Check if IP is in whitelist (comma-separated)
	for _, allowedIP := range strings.Split(whitelist, ",") {
		allowedIP = strings.TrimSpace(allowedIP)
		if allowedIP == "*" {
			return true, nil
		}
		// Check CIDR notation (e.g., 192.168.1.0/24)
		if strings.Contains(allowedIP, "/") {
			_, cidr, err := net.ParseCIDR(allowedIP)
			if err != nil {
				continue
			}
			if cidr.Contains(clientIP) {
				return true, nil
			}
		} else {
			// Check exact IP match
			if allowedIP == ip {
				return true, nil
			}
		}
	}

	return false, nil
}

// IsAccountExpired checks if an account has expired
func (s *AuthService) IsAccountExpired(ctx context.Context, userID int64) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var expiresAt sql.NullTime
	err := s.db.QueryRowContext(ctx, "SELECT expires_at FROM users WHERE id = ?", userID).Scan(&expiresAt)
	if err != nil {
		return false, err
	}

	if !expiresAt.Valid {
		return false, nil
	}

	return expiresAt.Time.Before(time.Now()), nil
}

