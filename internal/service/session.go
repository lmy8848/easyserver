package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"easyserver/internal/model"
	"easyserver/internal/repository"
)

type SessionService struct {
	sessionRepo     repository.SessionRepository
	activeThreshold time.Duration
}

// NewSessionService creates a SessionService using a repository
func NewSessionService(sessionRepo repository.SessionRepository) *SessionService {
	return &SessionService{
		sessionRepo:     sessionRepo,
		activeThreshold: 5 * time.Minute,
	}
}

// CreateSession creates a new session when user logs in
func (s *SessionService) CreateSession(ctx context.Context, token string, userID int64, username, role, ip, userAgent string, expiresAt time.Time) error {
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

// UpdateActivity updates the last_active timestamp for a session
// Returns error if session doesn't exist
func (s *SessionService) UpdateActivity(ctx context.Context, token string) error {
	return s.sessionRepo.UpdateActivity(ctx, token)
}

// RemoveSession removes a session (logout)
func (s *SessionService) RemoveSession(ctx context.Context, token string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.sessionRepo.DeleteByToken(ctx, token)
}

// RemoveUserSessions removes all sessions for a user (force logout)
func (s *SessionService) RemoveUserSessions(ctx context.Context, userID int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.sessionRepo.DeleteByUserID(ctx, userID)
}

// GetActiveSessions returns all active sessions
func (s *SessionService) GetActiveSessions(ctx context.Context) ([]model.Session, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.sessionRepo.GetActive(ctx)
}

// GetUserSessions returns active sessions for a specific user
func (s *SessionService) GetUserSessions(ctx context.Context, userID int64) ([]model.Session, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.sessionRepo.GetActiveByUserID(ctx, userID)
}

// CleanupExpiredSessions removes expired and inactive sessions
func (s *SessionService) CleanupExpiredSessions(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	// Remove expired sessions
	if err := s.sessionRepo.DeleteExpired(ctx); err != nil {
		return err
	}

	// Remove sessions inactive for more than 1 hour
	return s.sessionRepo.DeleteInactive(ctx, time.Now().Add(-1*time.Hour))
}

// GetSessionCount returns the count of active sessions
func (s *SessionService) GetSessionCount(ctx context.Context) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.sessionRepo.Count(ctx)
}

// IsSessionValid checks if a session exists and is not expired
// On database errors, returns false (invalid) to fail-closed for security
func (s *SessionService) IsSessionValid(ctx context.Context, token string) (bool, error) {
	valid, err := s.sessionRepo.IsValid(ctx, token)
	if err != nil {
		// Fail-closed: on DB error, treat session as invalid
		log.Printf("session: error checking session validity: %v", err)
		return false, err
	}
	return valid, nil
}

// GetActiveSessionsWithToken returns all active sessions with token (for current session identification)
func (s *SessionService) GetActiveSessionsWithToken(ctx context.Context) ([]model.Session, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.sessionRepo.GetActive(ctx)
}

// RemoveSessionByToken removes a specific session by token
func (s *SessionService) RemoveSessionByToken(ctx context.Context, token string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.sessionRepo.DeleteByToken(ctx, token)
}

// RemoveOtherSessions removes all sessions for a user except the current one
func (s *SessionService) RemoveOtherSessions(ctx context.Context, userID int64, currentToken string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.sessionRepo.DeleteByUserIDExcept(ctx, userID, currentToken)
}

// IsSessionValidByToken checks if a session exists by token (returns error if not found)
func (s *SessionService) IsSessionValidByToken(ctx context.Context, token string) (*model.Session, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	session, err := s.sessionRepo.GetByToken(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}
	return session, nil
}
