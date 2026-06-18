package service

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"easyserver/internal/model"
)

type SessionService struct {
	db          *sql.DB
	activeThreshold time.Duration // How long before a session is considered inactive
}

func NewSessionService(db *sql.DB) *SessionService {
	return &SessionService{
		db:              db,
		activeThreshold: 5 * time.Minute, // 5 minutes of inactivity = offline
	}
}

// CreateSession creates a new session when user logs in
func (s *SessionService) CreateSession(token string, userID int64, username, role, ip, userAgent string, expiresAt time.Time) error {
	_, err := s.db.Exec(
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
func (s *SessionService) UpdateActivity(token string) error {
	result, err := s.db.Exec(
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
func (s *SessionService) RemoveSession(token string) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE token = ?", token)
	return err
}

// RemoveUserSessions removes all sessions for a user (force logout)
func (s *SessionService) RemoveUserSessions(userID int64) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE user_id = ?", userID)
	return err
}

// GetActiveSessions returns all active sessions
func (s *SessionService) GetActiveSessions() ([]model.Session, error) {
	threshold := time.Now().Add(-s.activeThreshold)

	rows, err := s.db.Query(`
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
func (s *SessionService) GetUserSessions(userID int64) ([]model.Session, error) {
	threshold := time.Now().Add(-s.activeThreshold)

	rows, err := s.db.Query(`
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
func (s *SessionService) CleanupExpiredSessions() error {
	// Remove expired sessions
	_, err := s.db.Exec("DELETE FROM sessions WHERE expires_at < ?", time.Now())
	if err != nil {
		return err
	}

	// Remove sessions inactive for more than 1 hour
	_, err = s.db.Exec("DELETE FROM sessions WHERE last_active < ?", time.Now().Add(-1*time.Hour))
	return err
}

// GetSessionCount returns the count of active sessions
func (s *SessionService) GetSessionCount() (int, error) {
	threshold := time.Now().Add(-s.activeThreshold)
	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM sessions WHERE last_active > ? AND expires_at > ?",
		threshold, time.Now(),
	).Scan(&count)
	return count, err
}

// IsSessionValid checks if a session exists and is not expired
// On database errors, returns true (valid) to avoid blocking requests
func (s *SessionService) IsSessionValid(token string) (bool, error) {
	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM sessions WHERE token = ? AND expires_at > ?",
		token, time.Now(),
	).Scan(&count)
	if err != nil {
		// On database error, log and return true (valid)
		// This avoids blocking requests due to transient database issues
		log.Printf("session: error checking session validity: %v", err)
		return true, nil
	}
	return count > 0, nil
}
