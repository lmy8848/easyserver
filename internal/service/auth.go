package service

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"easyserver/internal/model"

	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	db            *sql.DB
	maxAttempts   int
	lockoutDuration time.Duration
}

func NewAuthService(db *sql.DB, maxAttempts int, lockoutDuration time.Duration) *AuthService {
	return &AuthService{
		db:              db,
		maxAttempts:     maxAttempts,
		lockoutDuration: lockoutDuration,
	}
}

func (s *AuthService) InitDefaultAdmin() error {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
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
		_, err = s.db.Exec(
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

// generateRandomPassword generates a random password of specified length
func generateRandomPassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	password := make([]byte, length)
	for i := range password {
		// Use crypto/rand for secure random
		b := make([]byte, 1)
		for {
			if _, err := rand.Read(b); err == nil {
				password[i] = charset[int(b[0])%len(charset)]
				break
			}
		}
	}
	return string(password)
}

func (s *AuthService) Login(username, password string) (*model.User, error) {
	user := &model.User{}
	var mustChangePass int

	err := s.db.QueryRow(
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

	if user.LockedUntil.Valid && user.LockedUntil.Time.After(time.Now()) {
		s.db.Exec("UPDATE users SET login_attempts = login_attempts + 1 WHERE id = ?", user.ID)
		return nil, errors.New("invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		// Atomic update: increment attempts and lock if threshold reached
		s.db.Exec(`UPDATE users SET
			login_attempts = login_attempts + 1,
			locked_until = CASE
				WHEN login_attempts + 1 >= ? THEN datetime('now', ?)
				ELSE locked_until
			END
			WHERE id = ?`, s.maxAttempts, fmt.Sprintf("+%d seconds", int(s.lockoutDuration.Seconds())), user.ID)
		return nil, errors.New("invalid credentials")
	}

	// Reset login attempts on success and update last login IP
	s.db.Exec("UPDATE users SET login_attempts = 0, locked_until = NULL, last_login = CURRENT_TIMESTAMP, last_login_ip = ? WHERE id = ?", "", user.ID)

	return user, nil
}

// LoginWithInfo logs in a user and records the login activity
func (s *AuthService) LoginWithInfo(username, password, ip, userAgent string) (*model.User, error) {
	user, err := s.Login(username, password)
	if err != nil {
		// Log failed login attempt
		s.LogUserActivity(0, username, "LOGIN_FAILED", ip, userAgent)
		return nil, err
	}

	// Check IP whitelist
	allowed, err := s.CheckIPWhitelist(user.ID, ip)
	if err != nil {
		return nil, err
	}
	if !allowed {
		s.LogUserActivity(user.ID, username, "LOGIN_BLOCKED_IP", ip, userAgent)
		return nil, errors.New("login not allowed from this IP")
	}

	// Check account expiry
	expired, err := s.IsAccountExpired(user.ID)
	if err != nil {
		return nil, err
	}
	if expired {
		s.LogUserActivity(user.ID, username, "LOGIN_BLOCKED_EXPIRED", ip, userAgent)
		return nil, errors.New("account has expired")
	}

	// Update last login IP
	s.db.Exec("UPDATE users SET last_login_ip = ? WHERE id = ?", ip, user.ID)

	// Log successful login
	s.LogUserActivity(user.ID, username, "LOGIN_SUCCESS", ip, userAgent)

	return user, nil
}

func (s *AuthService) ChangePassword(userID int64, oldPassword, newPassword string) error {
	var hash string
	var lockedUntil sql.NullTime
	err := s.db.QueryRow("SELECT password_hash, locked_until FROM users WHERE id = ?", userID).Scan(&hash, &lockedUntil)
	if err != nil {
		return err
	}

	// Check if account is locked
	if lockedUntil.Valid && lockedUntil.Time.After(time.Now()) {
		return errors.New("account is locked")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(oldPassword)); err != nil {
		return errors.New("invalid old password")
	}

	// Validate password strength
	// Simple validation: min 6 chars
	if len(newPassword) < 6 {
		return errors.New("password must be at least 6 characters")
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	_, err = s.db.Exec("UPDATE users SET password_hash = ?, must_change_pass = 0, updated_at = CURRENT_TIMESTAMP WHERE id = ?", string(newHash), userID)
	return err
}

func (s *AuthService) GetUserByID(id int64) (*model.User, error) {
	user := &model.User{}
	var lastLoginStr, createdAtStr sql.NullString
	var mustChangePass int
	err := s.db.QueryRow(
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

// AddTokenToBlacklist adds a token to the blacklist
func (s *AuthService) AddTokenToBlacklist(userID int64, token string, expiresAt time.Time) error {
	_, err := s.db.Exec(
		"INSERT INTO token_blacklist (user_id, token, expires_at) VALUES (?, ?, ?)",
		userID, token, expiresAt,
	)
	return err
}

// IsTokenBlacklisted checks if a token is blacklisted
func (s *AuthService) IsTokenBlacklisted(token string) (bool, error) {
	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM token_blacklist WHERE token = ? AND expires_at > ?",
		token, time.Now(),
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// InvalidateAllUserTokens invalidates all tokens for a user
// Stores the invalidation timestamp so tokens issued before this time are rejected
func (s *AuthService) InvalidateAllUserTokens(userID int64) error {
	// Delete any existing markers for this user first
	s.db.Exec("DELETE FROM token_blacklist WHERE user_id = ? AND token LIKE 'user_%_all'", userID)

	// Add a marker with the current time as the invalidation point
	// expires_at is set far in the future to keep the marker active
	_, err := s.db.Exec(
		"INSERT INTO token_blacklist (user_id, token, expires_at) VALUES (?, ?, ?)",
		userID, fmt.Sprintf("user_%d_all", userID), time.Now().Add(365*24*time.Hour),
	)
	return err
}

// IsUserTokenInvalidated checks if a user's tokens have been invalidated
// Returns true if the token was issued before the invalidation time
// On database errors, returns false (not invalidated) to avoid blocking requests
func (s *AuthService) IsUserTokenInvalidated(userID int64, issuedAt time.Time) (bool, error) {
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

	// Token is invalidated if it was issued before the invalidation marker
	return issuedAt.Before(createdAt), nil
}

// CleanupExpiredTokens removes expired tokens from blacklist
func (s *AuthService) CleanupExpiredTokens() error {
	_, err := s.db.Exec("DELETE FROM token_blacklist WHERE expires_at < ?", time.Now())
	return err
}

// ResetPassword resets a user's password (admin only)
func (s *AuthService) ResetPassword(userID int64, newPassword string) error {
	// Validate password strength
	if err := s.ValidatePassword(newPassword); err != nil {
		return err
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(
		"UPDATE users SET password_hash = ?, must_change_pass = 1, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		string(newHash), userID,
	)
	return err
}

// ValidatePassword validates password strength
func (s *AuthService) ValidatePassword(password string) error {
	if len(password) < 6 {
		return errors.New("password must be at least 6 characters")
	}
	if len(password) > 128 {
		return errors.New("password must be less than 128 characters")
	}
	return nil
}

// LogUserActivity logs a user activity
func (s *AuthService) LogUserActivity(userID int64, username, action, ip, userAgent string) error {
	_, err := s.db.Exec(
		"INSERT INTO user_activities (user_id, username, action, ip, user_agent) VALUES (?, ?, ?, ?, ?)",
		userID, username, action, ip, userAgent,
	)
	return err
}

// GetUserActivities returns user activity log
func (s *AuthService) GetUserActivities(userID int64, limit int) ([]model.UserActivity, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	rows, err := s.db.Query(
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
	return activities, nil
}

// GetAllActivities returns all user activities
func (s *AuthService) GetAllActivities(limit int) ([]model.UserActivity, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	rows, err := s.db.Query(
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
	return activities, nil
}

// SetAccountExpiry sets account expiration date
func (s *AuthService) SetAccountExpiry(userID int64, expiresAt *time.Time) error {
	if expiresAt == nil {
		_, err := s.db.Exec("UPDATE users SET expires_at = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ?", userID)
		return err
	}
	_, err := s.db.Exec("UPDATE users SET expires_at = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", expiresAt, userID)
	return err
}

// SetIPWhitelist sets IP whitelist for a user
func (s *AuthService) SetIPWhitelist(userID int64, whitelist string) error {
	_, err := s.db.Exec("UPDATE users SET ip_whitelist = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", whitelist, userID)
	return err
}

// CheckIPWhitelist checks if an IP is in the user's whitelist
func (s *AuthService) CheckIPWhitelist(userID int64, ip string) (bool, error) {
	var whitelist string
	err := s.db.QueryRow("SELECT ip_whitelist FROM users WHERE id = ?", userID).Scan(&whitelist)
	if err != nil {
		return false, err
	}

	// If whitelist is empty, allow all IPs
	if whitelist == "" {
		return true, nil
	}

	// Check if IP is in whitelist (comma-separated)
	// Simple check - in production, use proper IP parsing
	for _, allowedIP := range strings.Split(whitelist, ",") {
		allowedIP = strings.TrimSpace(allowedIP)
		if allowedIP == ip || allowedIP == "*" {
			return true, nil
		}
		// TODO: Support CIDR notation
	}

	return false, nil
}

// IsAccountExpired checks if an account has expired
func (s *AuthService) IsAccountExpired(userID int64) (bool, error) {
	var expiresAt sql.NullTime
	err := s.db.QueryRow("SELECT expires_at FROM users WHERE id = ?", userID).Scan(&expiresAt)
	if err != nil {
		return false, err
	}

	if !expiresAt.Valid {
		return false, nil
	}

	return expiresAt.Time.Before(time.Now()), nil
}

// GetActiveSessions returns all active sessions (from token blacklist)
func (s *AuthService) GetActiveSessions() ([]model.Session, error) {
	// Since we're using JWT, we don't have a session table
	// We can infer sessions from recent login activities
	rows, err := s.db.Query(`
		SELECT DISTINCT user_id, username, ip, user_agent, created_at
		FROM user_activities
		WHERE action = 'LOGIN_SUCCESS'
		AND created_at > datetime('now', '-24 hours')
		ORDER BY created_at DESC
		LIMIT 100
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []model.Session
	for rows.Next() {
		var s model.Session
		if err := rows.Scan(&s.UserID, &s.Username, &s.IP, &s.UserAgent, &s.LoginAt); err != nil {
			continue
		}
		s.ExpiresAt = s.LoginAt.Add(24 * time.Hour)
		sessions = append(sessions, s)
	}
	return sessions, nil
}
