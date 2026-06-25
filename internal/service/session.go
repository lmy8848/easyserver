package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"easyserver/internal/model"
	"easyserver/internal/repository"
)

type SessionService struct {
	sessionRepo     repository.SessionRepository
	db              *sql.DB // for operations not covered by repository
	activeThreshold time.Duration
}

func NewSessionService(db *sql.DB) *SessionService {
	return &SessionService{
		sessionRepo:     nil, // will be set via SetSessionRepository
		db:              db,
		activeThreshold: 5 * time.Minute,
	}
}

// NewSessionServiceWithRepo creates a SessionService using a repository
func NewSessionServiceWithRepo(sessionRepo repository.SessionRepository, db *sql.DB) *SessionService {
	return &SessionService{
		sessionRepo:     sessionRepo,
		db:              db,
		activeThreshold: 5 * time.Minute,
	}
}

// CreateSession creates a new session when user logs in
func (s *SessionService) CreateSession(ctx context.Context, token string, userID int64, username, role, ip, userAgent string, expiresAt time.Time) error {
	if s.sessionRepo != nil {
		return s.sessionRepo.Create(ctx, &model.Session{
			UserID:    userID,
			Username:  username,
			Role:      role,
			IP:        ip,
			UserAgent: userAgent,
			ExpiresAt: expiresAt,
			Token:     token,
		})
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (token, user_id, username, role, ip, user_agent, last_active, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		token, userID, username, role, ip, userAgent, time.Now(), expiresAt,
	)
	if err != nil {
		log.Printf("session: failed to create session: %v", err)
		return err
	}
	return nil
}

// UpdateActivity updates the last_active timestamp for a session
// Returns error if session doesn't exist
func (s *SessionService) UpdateActivity(ctx context.Context, token string) error {
	if s.sessionRepo != nil {
		return s.sessionRepo.UpdateActivity(ctx, token)
	}
	result, err := s.db.ExecContext(ctx,
		"UPDATE sessions SET last_active = ? WHERE token = ?",
		time.Now(), token,
	)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("session not found")
	}
	return nil
}

// RemoveSession removes a session (logout)
func (s *SessionService) RemoveSession(ctx context.Context, token string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if s.sessionRepo != nil {
		return s.sessionRepo.DeleteByToken(ctx, token)
	}
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE token = ?", token)
	return err
}

// RemoveUserSessions removes all sessions for a user (force logout)
func (s *SessionService) RemoveUserSessions(ctx context.Context, userID int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if s.sessionRepo != nil {
		return s.sessionRepo.DeleteByUserID(ctx, userID)
	}
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ?", userID)
	return err
}

// GetActiveSessions returns all active sessions
func (s *SessionService) GetActiveSessions(ctx context.Context) ([]model.Session, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s.sessionRepo != nil {
		return s.sessionRepo.GetActive(ctx)
	}
	threshold := time.Now().Add(-s.activeThreshold)

	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id, username, role, ip, user_agent, last_active, expires_at
		FROM sessions
		WHERE last_active > ? AND expires_at > ?
		ORDER BY last_active DESC
	`, threshold, time.Now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []model.Session
	for rows.Next() {
		var sess model.Session
		if err := rows.Scan(&sess.UserID, &sess.Username, &sess.Role, &sess.IP, &sess.UserAgent, &sess.LoginAt, &sess.ExpiresAt); err != nil {
			continue
		}
		sessions = append(sessions, sess)
	}
	return sessions, nil
}

// GetUserSessions returns active sessions for a specific user
func (s *SessionService) GetUserSessions(ctx context.Context, userID int64) ([]model.Session, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	threshold := time.Now().Add(-s.activeThreshold)

	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id, username, role, ip, user_agent, last_active, expires_at
		FROM sessions
		WHERE user_id = ? AND last_active > ? AND expires_at > ?
		ORDER BY last_active DESC
	`, userID, threshold, time.Now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []model.Session
	for rows.Next() {
		var sess model.Session
		if err := rows.Scan(&sess.UserID, &sess.Username, &sess.Role, &sess.IP, &sess.UserAgent, &sess.LoginAt, &sess.ExpiresAt); err != nil {
			continue
		}
		sessions = append(sessions, sess)
	}
	return sessions, nil
}

// CleanupExpiredSessions removes expired and inactive sessions
func (s *SessionService) CleanupExpiredSessions(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	// Remove expired sessions
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at < ?", time.Now())
	if err != nil {
		return err
	}

	// Remove sessions inactive for more than 1 hour
	_, err = s.db.ExecContext(ctx, "DELETE FROM sessions WHERE last_active < ?", time.Now().Add(-1*time.Hour))
	return err
}

// GetSessionCount returns the count of active sessions
func (s *SessionService) GetSessionCount(ctx context.Context) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s.sessionRepo != nil {
		return s.sessionRepo.Count(ctx)
	}
	threshold := time.Now().Add(-s.activeThreshold)
	var count int
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sessions WHERE last_active > ? AND expires_at > ?",
		threshold, time.Now(),
	).Scan(&count)
	return count, err
}

// IsSessionValid checks if a session exists and is not expired
// On database errors, returns false (invalid) to fail-closed for security
func (s *SessionService) IsSessionValid(ctx context.Context, token string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sessions WHERE token = ? AND expires_at > ?",
		token, time.Now(),
	).Scan(&count)
	if err != nil {
		// Fail-closed: on DB error, treat session as invalid
		log.Printf("session: error checking session validity: %v", err)
		return false, err
	}
	return count > 0, nil
}

// GetActiveSessionsWithToken returns all active sessions with token (for current session identification)
func (s *SessionService) GetActiveSessionsWithToken(ctx context.Context) ([]model.Session, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s.sessionRepo != nil {
		return s.sessionRepo.GetActive(ctx)
	}
	threshold := time.Now().Add(-s.activeThreshold)

	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id, username, role, ip, user_agent, last_active, expires_at, token
		FROM sessions
		WHERE last_active > ? AND expires_at > ?
		ORDER BY last_active DESC
	`, threshold, time.Now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []model.Session
	for rows.Next() {
		var sess model.Session
		if err := rows.Scan(&sess.UserID, &sess.Username, &sess.Role, &sess.IP, &sess.UserAgent, &sess.LoginAt, &sess.ExpiresAt, &sess.Token); err != nil {
			continue
		}
		sessions = append(sessions, sess)
	}
	return sessions, nil
}

// RemoveSessionByToken removes a specific session by token
func (s *SessionService) RemoveSessionByToken(ctx context.Context, token string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if s.sessionRepo != nil {
		return s.sessionRepo.DeleteByToken(ctx, token)
	}
	result, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE token = ?", token)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("session not found")
	}
	return nil
}

// RemoveOtherSessions removes all sessions for a user except the current one
func (s *SessionService) RemoveOtherSessions(ctx context.Context, userID int64, currentToken string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	_, err := s.db.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ? AND token != ?", userID, currentToken)
	return err
}
