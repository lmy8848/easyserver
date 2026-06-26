package auth

import (
	"context"
	"fmt"
	"log"
	"time"
)

type SessionService struct {
	sessionRepo     SessionRepo
	activeThreshold time.Duration
}

func NewSessionService(sessionRepo SessionRepo) *SessionService {
	return &SessionService{
		sessionRepo:     sessionRepo,
		activeThreshold: 5 * time.Minute,
	}
}

func (s *SessionService) CreateSession(ctx context.Context, token string, userID int64, username, role, ip, userAgent string, expiresAt time.Time) error {
	return s.sessionRepo.Create(ctx, &Session{
		UserID:    userID,
		Username:  username,
		Role:      role,
		IP:        ip,
		UserAgent: userAgent,
		ExpiresAt: expiresAt,
		Token:     token,
	})
}

func (s *SessionService) UpdateActivity(ctx context.Context, token string) error {
	return s.sessionRepo.UpdateActivity(ctx, token)
}

func (s *SessionService) RemoveSession(ctx context.Context, token string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.sessionRepo.DeleteByToken(ctx, token)
}

func (s *SessionService) RemoveUserSessions(ctx context.Context, userID int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.sessionRepo.DeleteByUserID(ctx, userID)
}

func (s *SessionService) GetActiveSessions(ctx context.Context) ([]Session, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.sessionRepo.GetActive(ctx)
}

func (s *SessionService) GetUserSessions(ctx context.Context, userID int64) ([]Session, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.sessionRepo.GetActiveByUserID(ctx, userID)
}

func (s *SessionService) CleanupExpiredSessions(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.sessionRepo.DeleteExpired(ctx); err != nil {
		return err
	}
	return s.sessionRepo.DeleteInactive(ctx, time.Now().Add(-1*time.Hour))
}

func (s *SessionService) GetSessionCount(ctx context.Context) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.sessionRepo.Count(ctx)
}

func (s *SessionService) IsSessionValid(ctx context.Context, token string) (bool, error) {
	valid, err := s.sessionRepo.IsValid(ctx, token)
	if err != nil {
		log.Printf("session: error checking session validity: %v", err)
		return false, err
	}
	return valid, nil
}

func (s *SessionService) GetActiveSessionsWithToken(ctx context.Context) ([]Session, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.sessionRepo.GetActive(ctx)
}

func (s *SessionService) RemoveSessionByToken(ctx context.Context, token string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.sessionRepo.DeleteByToken(ctx, token)
}

func (s *SessionService) RemoveOtherSessions(ctx context.Context, userID int64, currentToken string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.sessionRepo.DeleteByUserIDExcept(ctx, userID, currentToken)
}

func (s *SessionService) IsSessionValidByToken(ctx context.Context, token string) (*Session, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	session, err := s.sessionRepo.GetByToken(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}
	return session, nil
}
