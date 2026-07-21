package auth

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
)

type SessionService struct {
	sessionRepo     SessionRepo
	activeThreshold time.Duration
	// mobileMu serializes mobile-session create/replace per process so the
	// single-device binding check + create is atomic (prevents two mobile
	// logins racing past an empty check). Single-admin panel -> one mutex.
	mobileMu sync.Mutex
}

func NewSessionService(sessionRepo SessionRepo) *SessionService {
	return &SessionService{
		sessionRepo:     sessionRepo,
		activeThreshold: 5 * time.Minute,
	}
}

func (s *SessionService) CreateSession(ctx context.Context, token string, userID int64, username, role, ip, userAgent, clientType, deviceID, deviceInfo string, expiresAt time.Time) error {
	return s.sessionRepo.Create(ctx, &Session{
		UserID:     userID,
		Username:   username,
		Role:       role,
		IP:         ip,
		UserAgent:  userAgent,
		ClientType: clientType,
		DeviceID:   deviceID,
		DeviceInfo: deviceInfo,
		ExpiresAt:  expiresAt,
		Token:      token,
	})
}

// ErrMobileDeviceBound is returned by CreateMobileSessionBound when an active
// mobile session exists on a different device_id; the caller rejects the login.
var ErrMobileDeviceBound = errors.New("mobile device bound by another session")

// CreateMobileSessionBound creates a mobile session enforcing single-device
// binding, atomically (process-locked). If an active mobile session exists on
// the same device_id, the new session is created FIRST and the old one removed
// after (so a create failure does NOT lock the user out). On a different
// device_id it returns ErrMobileDeviceBound and creates nothing.
//
// device_id is a client-reported soft identifier, not a security boundary; the
// real control is "one active mobile session + panel-revocable" (see
// docs/mobile-app-plan.md §4.1.1).
func (s *SessionService) CreateMobileSessionBound(ctx context.Context, session *Session) error {
	if ctx == nil {
		ctx = context.Background()
	}
	s.mobileMu.Lock()
	defer s.mobileMu.Unlock()

	existing, err := s.activeMobileSession(ctx, session.UserID)
	if err != nil {
		return err
	}
	if existing != nil {
		if session.DeviceID != "" && existing.DeviceID == session.DeviceID {
			// Same device: create new first, then remove the old (avoid lockout on create failure).
			if err := s.sessionRepo.Create(ctx, session); err != nil {
				return err
			}
			return s.sessionRepo.DeleteMobileByUserIDExcept(ctx, session.UserID, session.Token)
		}
		return ErrMobileDeviceBound
	}
	return s.sessionRepo.Create(ctx, session)
}

// activeMobileSession returns the user's active mobile session, or nil if none.
func (s *SessionService) activeMobileSession(ctx context.Context, userID int64) (*Session, error) {
	sessions, err := s.sessionRepo.GetActiveByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range sessions {
		if sessions[i].ClientType == "mobile" {
			return &sessions[i], nil
		}
	}
	return nil, nil
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

// RemoveSessionByStoredToken deletes a session by its already-hashed stored
// token (no re-hashing). Callers pass the value returned by GetSessions.
func (s *SessionService) RemoveSessionByStoredToken(ctx context.Context, storedToken string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.sessionRepo.DeleteByStoredToken(ctx, storedToken)
}

// RemoveMobileSessions deletes all mobile sessions for a user. Used by the
// same-device refresh path to replace the bound mobile session.
func (s *SessionService) RemoveMobileSessions(ctx context.Context, userID int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.sessionRepo.DeleteMobileByUserID(ctx, userID)
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
